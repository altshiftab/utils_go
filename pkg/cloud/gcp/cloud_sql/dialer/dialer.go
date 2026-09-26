package dialer

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql/dialer/dialer_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql/types/connect_settings"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql/types/connection_name"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token_source"
)

const keyBits = 2048

type connectionInfo struct {
	address   string
	tlsConfig *tls.Config
	expiry    time.Time
}

type entry struct {
	mu   sync.Mutex
	info *connectionInfo
}

// Dialer connects to Cloud SQL instances the way the Cloud SQL Auth Proxy does: TLS 1.3 to the
// instance's server-side proxy, authenticated by an ephemeral client certificate from the Admin
// API. The returned connection speaks the database protocol directly, so a Postgres driver dials
// through it with its own TLS turned off.
type Dialer struct {
	client *cloud_sql.Client
	// loginTokenSource, when set, makes every certificate carry a login token for its principal.
	loginTokenSource token_source.TokenSource
	key              *rsa.PrivateKey
	publicKeyPem     string
	config           *dialer_config.Config

	mu      sync.Mutex
	entries map[string]*entry
}

func New(client *cloud_sql.Client, loginTokenSource token_source.TokenSource, options ...dialer_config.Option) (*Dialer, error) {
	if client == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("cloud sql client"))
	}

	key, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("rsa generate key: %w", err))
	}

	publicKeyDer, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("x509 marshal pkix public key: %w", err))
	}

	return &Dialer{
		client:           client,
		loginTokenSource: loginTokenSource,
		key:              key,
		// The API wants this block type around a PKIX key, as the Google connectors send it.
		publicKeyPem: string(pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: publicKeyDer})),
		config:       dialer_config.New(options...),
		entries:      make(map[string]*entry),
	}, nil
}

func (d *Dialer) entry(key string) *entry {
	d.mu.Lock()
	defer d.mu.Unlock()

	e, ok := d.entries[key]
	if !ok {
		e = &entry{}
		d.entries[key] = e
	}

	return e
}

func (d *Dialer) connectionInfo(ctx context.Context, connectionName *connection_name.ConnectionName) (*connectionInfo, error) {
	e := d.entry(connectionName.String())

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.info != nil && d.config.Now().Add(d.config.RefreshBuffer).Before(e.info.expiry) {
		return e.info, nil
	}

	info, err := d.fetchConnectionInfo(ctx, connectionName)
	if err != nil {
		return nil, err
	}
	e.info = info

	return info, nil
}

func (d *Dialer) invalidate(connectionName *connection_name.ConnectionName) {
	e := d.entry(connectionName.String())
	e.mu.Lock()
	e.info = nil
	e.mu.Unlock()
}

func (d *Dialer) fetchConnectionInfo(ctx context.Context, connectionName *connection_name.ConnectionName) (*connectionInfo, error) {
	settings, err := d.client.GetConnectSettings(ctx, connectionName.Project, connectionName.Instance)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("get connect settings: %w", err), connectionName.String())
	}
	if settings.BackendType != "" && settings.BackendType != connect_settings.BackendTypeSecondGen {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: unsupported backend type %q", altshiftErrors.ErrValidationError, settings.BackendType),
			connectionName.String(),
		)
	}
	if settings.Region != connectionName.Region {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: instance is in region %q", altshiftErrors.ErrValidationError, settings.Region),
			connectionName.String(),
		)
	}

	address, err := selectAddress(settings, d.config.IpType)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("select address: %w", err), connectionName.String())
	}

	roots, err := parseCaCertificates(settings)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("parse ca certificates: %w", err), connectionName.String())
	}

	var accessToken string
	var tokenExpiry time.Time
	if d.loginTokenSource != nil {
		token, err := d.loginTokenSource.Token()
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("login token source token: %w", err))
		}
		if token == nil || token.AccessToken == "" {
			return nil, altshiftErrors.NewWithTrace(empty_error.New("login access token"))
		}
		accessToken, tokenExpiry = token.AccessToken, token.Expiry
	}

	response, err := d.client.GenerateEphemeralCert(ctx, connectionName.Project, connectionName.Instance, d.publicKeyPem, accessToken)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("generate ephemeral cert: %w", err), connectionName.String())
	}

	block, _ := pem.Decode([]byte(response.EphemeralCert.Cert))
	if block == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("ephemeral cert pem block"))
	}
	clientCertificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("x509 parse certificate (ephemeral cert): %w", err))
	}

	// The database stops accepting the certificate's login once its token expires.
	expiry := clientCertificate.NotAfter
	if !tokenExpiry.IsZero() && tokenExpiry.Before(expiry) {
		expiry = tokenExpiry
	}

	serverName := settings.DnsName
	if len(settings.DnsNames) > 0 && settings.DnsNames[0] != nil && settings.DnsNames[0].Name != "" {
		serverName = settings.DnsNames[0].Name
	}

	return &connectionInfo{
		address: net.JoinHostPort(address, d.config.Port),
		tlsConfig: &tls.Config{
			ServerName: serverName,
			Certificates: []tls.Certificate{{
				Certificate: [][]byte{clientCertificate.Raw},
				PrivateKey:  d.key,
				Leaf:        clientCertificate,
			}},
			RootCAs:    roots,
			MinVersion: tls.VersionTLS13,
			// Replaced by verifyConnectionFunc, which also accepts the legacy CN-only certificates.
			InsecureSkipVerify: true, //nolint:gosec // G402: verification is done by VerifyConnection
			VerifyConnection:   verifyConnectionFunc(serverName, connectionName, roots),
		},
		expiry: expiry,
	}, nil
}

func selectAddress(settings *connect_settings.ConnectSettings, ipType string) (string, error) {
	want := connect_settings.IpAddressTypePrimary
	if ipType == dialer_config.IpTypePrivate {
		want = connect_settings.IpAddressTypePrivate
	}

	for _, mapping := range settings.IpAddresses {
		if mapping != nil && mapping.Type == want && mapping.IpAddress != "" {
			return mapping.IpAddress, nil
		}
	}

	return "", altshiftErrors.NewWithTrace(
		fmt.Errorf("%w: instance has no %s address", altshiftErrors.ErrValidationError, ipType),
	)
}

func parseCaCertificates(settings *connect_settings.ConnectSettings) (*x509.CertPool, error) {
	if settings.ServerCaCert == nil || settings.ServerCaCert.Cert == "" {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("server ca cert"))
	}

	pool := x509.NewCertPool()
	var count int
	rest := []byte(settings.ServerCaCert.Cert)
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("x509 parse certificate (server ca): %w", err))
		}
		pool.AddCert(certificate)
		count++
	}
	if count == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("server ca certificates"))
	}

	return pool, nil
}

// Dial returns a handshaken TLS connection to the instance named by connectionName
// (`project:region:instance`).
func (d *Dialer) Dial(ctx context.Context, connectionNameString string) (net.Conn, error) {
	connectionName, err := connection_name.Parse(connectionNameString)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("connection name parse: %w", err), connectionNameString)
	}

	info, err := d.connectionInfo(ctx, connectionName)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("connection info: %w", err), connectionNameString)
	}

	conn, err := d.config.NetDialer.DialContext(ctx, "tcp", info.address)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("net dialer dial context: %w", err), info.address)
	}

	tlsConn := tls.Client(conn, info.tlsConfig)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		// A rotated server CA or a revoked certificate is fixed by fetching both again; a caller
		// that gave up says nothing about either.
		if ctx.Err() == nil {
			d.invalidate(connectionName)
		}
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("tls conn handshake context: %w", err), info.address)
	}

	return tlsConn, nil
}
