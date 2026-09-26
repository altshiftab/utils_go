// Command cloudsql_proxy forwards local TCP connections to a Cloud SQL instance the way the Cloud SQL
// Auth Proxy does, logging in with IAM database authentication as the principal of the credentials:
// GOOGLE_OAUTH_ACCESS_TOKEN when set, Application Default Credentials otherwise. Given a command
// after "--", it runs the command with the proxy listening and PGHOST, PGPORT, PGUSER and PGSSLMODE
// set for it, and exits with the command's status.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	argumentParser "github.com/altshiftab/utils_go/pkg/cli/argument_parser"
	argumentParserErrors "github.com/altshiftab/utils_go/pkg/cli/argument_parser/errors"
	"github.com/altshiftab/utils_go/pkg/cli/argument_parser/option"
	altshiftGcp "github.com/altshiftab/utils_go/pkg/cloud/gcp"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql/cloud_sql_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql/dialer"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql/dialer/dialer_config"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config/retry_config"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token_source"
	altshiftOauth2Transport "github.com/altshiftab/utils_go/pkg/oauth2/types/transport"
)

const (
	exitClean = 0
	exitError = 1
	exitUsage = 2

	programName    = "cloudsql_proxy"
	accessTokenEnv = "GOOGLE_OAUTH_ACCESS_TOKEN" //nolint:gosec // G101: the name of a variable, not a credential
)

const description = "Forward local connections to a Cloud SQL instance through its server-side proxy, logging in " +
	"with IAM database authentication as the principal of GOOGLE_OAUTH_ACCESS_TOKEN or of Application Default " +
	"Credentials; no password and no authorized network are involved. With a command after --, run it with the " +
	"proxy listening and PGHOST, PGPORT, PGUSER and PGSSLMODE set, and exit with its status."

type arguments struct {
	instance string
	address  string
	port     int
	ipType   string
	command  []string
}

func newParser(args *arguments, output io.Writer) *argumentParser.Parser {
	return &argumentParser.Parser{
		ProgramName: programName,
		Description: description,
		Output:      output,
		Options: []option.Option{
			option.WithMetavar(option.WithDefault(option.NewStringOption(0, "address", "the local address to listen on", false, &args.address), "127.0.0.1"), "ADDRESS"),
			option.WithMetavar(option.WithDefault(option.NewIntOption(0, "port", "the local port to listen on; 0 picks a free one", false, &args.port), "5433"), "PORT"),
			option.WithChoices(option.WithDefault(option.NewStringOption(0, "ip-type", "which of the instance's addresses to dial", false, &args.ipType), dialer_config.IpTypePublic), dialer_config.IpTypePublic, dialer_config.IpTypePrivate),
		},
		Positionals: []option.Option{
			option.WithMetavar(option.NewStringOption(0, "", "the instance's connection name, project:region:instance", true, &args.instance), "INSTANCE"),
		},
		Rest:          &args.command,
		DisableAbbrev: true,
	}
}

// dialFunc opens a connection to the instance, speaking the Postgres protocol.
type dialFunc func(ctx context.Context) (net.Conn, error)

// tokenSources returns the token source for the Admin API and the one whose token goes into the
// client certificate. They refresh long after ctx may end, so they keep its values without its
// cancellation.
func tokenSources(ctx context.Context) (token_source.TokenSource, token_source.TokenSource, error) {
	ctx = context.WithoutCancel(ctx)

	if accessToken := os.Getenv(accessTokenEnv); accessToken != "" {
		// The token's expiry is not known; the certificate is replaced well within an hour.
		ts := token_source.NewStatic(&token.Token{AccessToken: accessToken, Expiry: time.Now().Add(30 * time.Minute)})
		return ts, ts, nil
	}

	gcpClient := altshiftGcp.NewClient()
	adminTokenSource, err := gcpClient.FindDefaultCredentials(ctx, []string{cloud_sql.AdminScope})
	if err != nil {
		return nil, nil, altshiftErrors.New(fmt.Errorf("find default credentials (admin): %w", err))
	}
	loginTokenSource, err := gcpClient.FindDefaultCredentials(ctx, []string{cloud_sql.LoginScope})
	if err != nil {
		return nil, nil, altshiftErrors.New(fmt.Errorf("find default credentials (login): %w", err))
	}

	return adminTokenSource, loginTokenSource, nil
}

// serve forwards every connection the listener accepts to one dial opens, until the listener is closed.
func serve(ctx context.Context, listener net.Listener, dial dialFunc, stderr io.Writer) {
	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		client, err := listener.Accept()
		if err != nil {
			return
		}

		wg.Go(func() {
			defer func() { _ = client.Close() }()

			server, err := dial(ctx)
			if err != nil {
				fmt.Fprintf(stderr, "%s: error: dial: %v\n", programName, err)
				return
			}
			defer func() { _ = server.Close() }()

			pipe(client, server)
		})
	}
}

// pipe copies both ways until either side finishes, then closes both so the other copy ends too.
func pipe(a net.Conn, b net.Conn) {
	var once sync.Once
	closeBoth := func() {
		once.Do(func() {
			_ = a.Close()
			_ = b.Close()
		})
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		_, _ = io.Copy(a, b)
		closeBoth()
	})
	wg.Go(func() {
		_, _ = io.Copy(b, a)
		closeBoth()
	})
	wg.Wait()
}

// runCommand runs command with the proxy's address and the login username in its environment.
func runCommand(ctx context.Context, command []string, address net.Addr, username string, stdout, stderr io.Writer) (int, error) {
	host, port, err := net.SplitHostPort(address.String())
	if err != nil {
		return exitError, altshiftErrors.NewWithTrace(fmt.Errorf("net split host port: %w", err), address.String())
	}

	cmd := exec.CommandContext(ctx, command[0], command[1:]...) //nolint:gosec // G204: running the caller's command is the point
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = append(os.Environ(), "PGHOST="+host, "PGPORT="+port, "PGUSER="+username, "PGSSLMODE=disable")

	if err := cmd.Run(); err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			return exitErr.ExitCode(), nil
		}
		return exitError, altshiftErrors.NewWithTrace(fmt.Errorf("exec cmd run: %w", err), command)
	}

	return exitClean, nil
}

// proxy listens, then serves until ctx ends or, with a command, until the command exits.
func proxy(ctx context.Context, args *arguments, dial dialFunc, username string, stdout, stderr io.Writer) (int, error) {
	address := net.JoinHostPort(args.address, strconv.Itoa(args.port))
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", address)
	if err != nil {
		return exitError, altshiftErrors.NewWithTrace(fmt.Errorf("listen: %w", err), address)
	}

	done := make(chan struct{})
	go func() {
		serve(ctx, listener, dial, stderr)
		close(done)
	}()
	stop := func() {
		_ = listener.Close()
		<-done
	}

	if len(args.command) == 0 {
		fmt.Fprintf(stderr, "%s: forwarding %s to %s as %s\n", programName, listener.Addr(), args.instance, username)
		<-ctx.Done()
		stop()
		return exitClean, nil
	}

	code, err := runCommand(ctx, args.command, listener.Addr(), username, stdout, stderr)
	stop()

	return code, err
}

// run does what the command line asks and returns the exit code and, for a failure, the error to report.
func run(ctx context.Context, argv []string, stdout, stderr io.Writer) (int, error) {
	args := &arguments{}
	parser := newParser(args, stdout)
	if err := parser.Validate(); err != nil {
		return exitError, altshiftErrors.New(fmt.Errorf("parser validate: %w", err))
	}
	if err := parser.ParseArgs(argv); err != nil {
		if errors.Is(err, argumentParserErrors.ErrHelp) {
			return exitClean, nil
		}
		fmt.Fprint(stderr, parser.FormatError(err))
		return exitUsage, nil
	}

	adminTokenSource, loginTokenSource, err := tokenSources(ctx)
	if err != nil {
		return exitError, err
	}

	username, err := cloud_sql.NewUsernameResolver().Resolve(ctx, adminTokenSource)
	if err != nil {
		return exitError, fmt.Errorf("resolve username: %w", err)
	}

	client := cloud_sql.NewClient(cloud_sql_config.WithFetchOptions(
		fetch_config.WithHttpClient(&http.Client{Transport: &altshiftOauth2Transport.Transport{Source: adminTokenSource}}),
		fetch_config.WithRetryConfig(retry_config.New(
			retry_config.WithCount(4),
			retry_config.WithBaseDelay(time.Second),
			retry_config.WithMaximumWaitTime(30*time.Second),
		)),
	))
	d, err := dialer.New(client, loginTokenSource, dialer_config.WithIpType(args.ipType))
	if err != nil {
		return exitError, fmt.Errorf("dialer new: %w", err)
	}

	return proxy(ctx, args, func(ctx context.Context) (net.Conn, error) { return d.Dial(ctx, args.instance) }, username, stdout, stderr)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	code, err := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: error: %v\n", programName, err)
	}
	os.Exit(code)
}
