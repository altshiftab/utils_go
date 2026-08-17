package service

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"syscall"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	muxResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	muxResponseError "github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/service/service_config"
)

// get performs a request the linters accept: with a context, and closing the body.
func get(t *testing.T, url string) (string, error) {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}
	if response == nil {
		return "", nil_error.New("response")
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Errorf("response body close: %v", err)
		}
	}()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// waitUntilServing waits for the service to answer at the address, so that a test that stops it can
// tell "it stopped" from "it had not started".
func waitUntilServing(t *testing.T, address string) {
	t.Helper()

	var err error
	for range 100 {
		if _, err = get(t, "http://"+address); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("the service never started serving: %v", err)
}

func listen(t *testing.T) net.Listener {
	t.Helper()

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen config listen: %v", err)
	}

	return listener
}

func TestServeListenerFinishesRequestsBeingHandled(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})

	service, err := New(
		service_config.WithEndpoints(
			&endpoint.Endpoint{
				Path:   "/",
				Method: http.MethodGet,
				Handler: func(_ *http.Request, _ []byte) (*muxResponse.Response, *muxResponseError.ResponseError) {
					close(started)
					<-release
					return &muxResponse.Response{StatusCode: http.StatusOK, Body: []byte("finished")}, nil
				},
			},
		),
		service_config.WithShutdownTimeout(5*time.Second),
	)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	listener := listen(t)
	address := listener.Addr().String()

	ctx, cancel := context.WithCancel(context.WithoutCancel(t.Context()))
	defer cancel()

	served := make(chan error, 1)
	go func() { served <- service.ServeListener(ctx, listener) }()

	responses := make(chan string, 1)
	go func() {
		body, err := get(t, "http://"+address)
		if err != nil {
			responses <- "error: " + err.Error()
			return
		}
		responses <- body
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the request was never handled")
	}

	// Asking the service to stop while a request is in flight must not end the request.
	cancel()
	close(release)

	select {
	case body := <-responses:
		if body != "finished" {
			t.Errorf("response: got %q, want %q", body, "finished")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the request never finished")
	}

	select {
	case err := <-served:
		if err != nil {
			t.Errorf("serve listener: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serving never stopped")
	}
}

func TestServeListenerStopsAcceptingAfterBeingAskedToStop(t *testing.T) {
	t.Parallel()

	service, err := New(service_config.WithEndpoints(noContentEndpoint()), service_config.WithShutdownTimeout(time.Second))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	listener := listen(t)
	address := listener.Addr().String()

	ctx, cancel := context.WithCancel(context.WithoutCancel(t.Context()))
	defer cancel()

	served := make(chan error, 1)
	go func() { served <- service.ServeListener(ctx, listener) }()

	waitUntilServing(t, address)

	cancel()

	select {
	case err := <-served:
		if err != nil {
			t.Errorf("serve listener: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serving never stopped")
	}

	if _, err := get(t, "http://"+address); err == nil {
		t.Error("the service still accepts requests")
	}
}

// TestServeListenerReportsShutdownTimeout verifies that a request that does not finish within the
// time it is given makes the shutdown report it, rather than the service stopping as though the
// request had finished.
func TestServeListenerReportsShutdownTimeout(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	service, err := New(
		service_config.WithEndpoints(
			&endpoint.Endpoint{
				Path:   "/",
				Method: http.MethodGet,
				Handler: func(_ *http.Request, _ []byte) (*muxResponse.Response, *muxResponseError.ResponseError) {
					close(started)
					<-release
					return &muxResponse.Response{StatusCode: http.StatusNoContent}, nil
				},
			},
		),
		service_config.WithShutdownTimeout(100*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	listener := listen(t)
	address := listener.Addr().String()

	ctx, cancel := context.WithCancel(context.WithoutCancel(t.Context()))
	defer cancel()

	served := make(chan error, 1)
	go func() { served <- service.ServeListener(ctx, listener) }()

	// The request is made without the testing helpers, which must not be reached from a goroutine
	// that outlives the test.
	go func() {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+address, nil)
		if err != nil {
			return
		}

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return
		}

		_ = response.Body.Close()
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the request was never handled")
	}

	cancel()

	select {
	case err := <-served:
		if err == nil {
			t.Fatal("serve listener: got no error, want one")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("serve listener: got %v, want a deadline exceeded error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serving never stopped")
	}
}

func TestServeListenerReportsServeErrors(t *testing.T) {
	t.Parallel()

	service, err := New(service_config.WithEndpoints(noContentEndpoint()))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	listener := listen(t)
	if err := listener.Close(); err != nil {
		t.Fatalf("listener close: %v", err)
	}

	err = service.ServeListener(context.WithoutCancel(t.Context()), listener)
	if err == nil {
		t.Fatal("serve listener: got no error, want one")
	}
	if errors.Is(err, http.ErrServerClosed) {
		t.Errorf("serve listener: got %v", err)
	}
}

func TestServeListenerWithoutListener(t *testing.T) {
	t.Parallel()

	service, err := New(service_config.WithEndpoints(noContentEndpoint()))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	if err := service.ServeListener(t.Context(), nil); err == nil {
		t.Error("serve listener: got no error, want one")
	}
}

func TestServeWithoutServer(t *testing.T) {
	t.Parallel()

	service := &Service{}

	if err := service.ServeContext(t.Context()); err == nil {
		t.Error("serve context: got no error, want one")
	}

	if err := service.ServeListener(t.Context(), listen(t)); err == nil {
		t.Error("serve listener: got no error, want one")
	}
}

func TestServeContextWithoutAddress(t *testing.T) {
	t.Parallel()

	service, err := New(service_config.WithEndpoints(noContentEndpoint()))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	// An address is required rather than left to the standard library, whose empty address means
	// the privileged port 80 -- not something to bind by accident.
	if err := service.ServeContext(t.Context()); err == nil {
		t.Error("serve context: got no error, want one")
	}
}

func TestServeContextReportsListenErrors(t *testing.T) {
	t.Parallel()

	service, err := New(service_config.WithEndpoints(noContentEndpoint()), service_config.WithAddress("127.0.0.1:not-a-port"))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	if err := service.ServeContext(t.Context()); err == nil {
		t.Error("serve context: got no error, want one")
	}
}

func TestServeContextStopsWhenTheContextIsCancelled(t *testing.T) {
	t.Parallel()

	// The port is picked by the operating system and then given up, so that the service can listen
	// on it itself.
	listener := listen(t)
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("listener close: %v", err)
	}

	service, err := New(service_config.WithEndpoints(noContentEndpoint()), service_config.WithAddress(address))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	ctx, cancel := context.WithCancel(context.WithoutCancel(t.Context()))
	defer cancel()

	served := make(chan error, 1)
	go func() { served <- service.ServeContext(ctx) }()

	waitUntilServing(t, address)

	cancel()

	select {
	case err := <-served:
		if err != nil {
			t.Errorf("serve context: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serving never stopped")
	}
}

// TestServeStopsOnSignal verifies that the process being asked to stop stops the service, which is
// what Serve adds over ServeContext.
func TestServeStopsOnSignal(t *testing.T) {
	t.Parallel()

	listener := listen(t)
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("listener close: %v", err)
	}

	// SIGUSR1 stands in for the SIGTERM a supervisor sends, which the test process must not be
	// asked to handle for real.
	service, err := New(
		service_config.WithEndpoints(noContentEndpoint()),
		service_config.WithAddress(address),
		service_config.WithSignals(syscall.SIGUSR1),
	)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	served := make(chan error, 1)
	go func() { served <- service.Serve() }()

	// Serve installs the signal handling before it starts listening, so a service that answers is
	// one whose handling is in place -- the signal cannot end the test process instead.
	waitUntilServing(t, address)

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGUSR1); err != nil {
		t.Fatalf("syscall kill: %v", err)
	}

	select {
	case err := <-served:
		if err != nil {
			t.Errorf("serve: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serving never stopped")
	}
}
