package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kwhitestone/prism-fusion/plugin"
)

type applicationTestAddon struct {
	plugin.BasePlugin
	event func(string) error
	ctx   context.Context
}

func (p *applicationTestAddon) Validate(context.Context) error { return p.event("validate") }
func (p *applicationTestAddon) Initialize(ctx context.Context) error {
	p.ctx = ctx
	return p.event("initialize")
}
func (p *applicationTestAddon) Start(context.Context) error { return p.event("start") }
func (p *applicationTestAddon) Ready(context.Context) error { return p.event("ready") }
func (p *applicationTestAddon) Stop(ctx context.Context) error {
	if p.ctx.Err() == nil {
		return errors.New("worker context was not canceled")
	}
	if ctx.Err() != nil {
		return errors.New("stop context already canceled")
	}
	return p.event("stop")
}

type applicationCloser struct{ close func() error }

func (c applicationCloser) Close() error { return c.close() }

func applicationFixture(t *testing.T, failure, panicAt string) (applicationOperations, func() []string, *applicationTestAddon) {
	t.Helper()
	var mu sync.Mutex
	events := []string{}
	event := func(phase string) error {
		mu.Lock()
		events = append(events, phase)
		mu.Unlock()
		if phase == panicAt {
			panic("startup panic")
		}
		if phase == failure {
			return fmt.Errorf("failed %s", phase)
		}
		return nil
	}
	addon := &applicationTestAddon{BasePlugin: plugin.BasePlugin{PluginName: "test"}, event: event}
	registry := plugin.NewRegistry()
	if err := registry.Register(addon); err != nil {
		t.Fatal(err)
	}
	operations := applicationOperations{
		configure: func() error { return event("configure") },
		openDatabase: func() (io.Closer, error) {
			if err := event("database"); err != nil {
				return nil, err
			}
			return applicationCloser{func() error { return event("close-database") }}, nil
		},
		migrate: func() error { return event("migrate") },
		router: func() (http.Handler, error) {
			if err := event("router"); err != nil {
				return nil, err
			}
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }), nil
		},
		address: func() string { return "127.0.0.1:0" },
		listen: func(network, address string) (net.Listener, error) {
			if err := event("listen"); err != nil {
				return nil, err
			}
			return net.Listen(network, address)
		},
		lifecycle: plugin.NewLifecycle(registry),
	}
	return operations, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), events...) }, addon
}

func TestApplicationStartupFailuresReleaseResources(t *testing.T) {
	for _, phase := range []string{"configure", "database", "validate", "initialize", "migrate", "router", "start", "ready", "listen"} {
		t.Run(phase, func(t *testing.T) {
			ops, events, _ := applicationFixture(t, phase, "")
			err := runApplication(context.Background(), ApplicationOptions{}, ops)
			if err == nil || !strings.Contains(err.Error(), phase) {
				t.Fatalf("error = %v", err)
			}
			all := events()
			if phase != "listen" && containsEvent(all, "listen") {
				t.Fatal("served before ready")
			}
			if phase != "configure" && phase != "database" && all[len(all)-1] != "close-database" {
				t.Fatalf("database not closed last: %v", all)
			}
			if phase != "configure" && phase != "database" && phase != "validate" && !containsEvent(all, "stop") {
				t.Fatalf("addon not stopped: %v", all)
			}
		})
	}
}

func containsEvent(events []string, event string) bool {
	for _, value := range events {
		if value == event {
			return true
		}
	}
	return false
}

func TestApplicationPanicRollsBack(t *testing.T) {
	ops, events, _ := applicationFixture(t, "", "router")
	err := runApplication(context.Background(), ApplicationOptions{}, ops)
	if err == nil || !strings.Contains(err.Error(), "panic") {
		t.Fatalf("error = %v", err)
	}
	all := events()
	if !reflect.DeepEqual(all[len(all)-2:], []string{"stop", "close-database"}) {
		t.Fatalf("cleanup order = %v", all)
	}
}

func TestApplicationNoDatabaseAndGracefulHTTPShutdown(t *testing.T) {
	ops, events, addon := applicationFixture(t, "", "")
	listening := make(chan string, 1)
	startRequest, releaseRequest := make(chan struct{}), make(chan struct{})
	ops.router = func() (http.Handler, error) {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(startRequest)
			<-releaseRequest
			if addon.ctx.Err() != nil {
				t.Error("workers canceled before request drained")
			}
			w.WriteHeader(http.StatusNoContent)
		}), nil
	}
	listen := ops.listen
	ops.listen = func(network, address string) (net.Listener, error) {
		listener, err := listen(network, address)
		if err == nil {
			listening <- listener.Addr().String()
		}
		return listener, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runApplication(ctx, ApplicationOptions{DisableDatabase: true}, ops) }()
	var address string
	select {
	case address = <-listening:
	case err := <-done:
		t.Fatalf("server failed before listening: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("server did not listen")
	}
	requestDone := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: time.Second}
		response, err := client.Get("http://" + address + "/health")
		if err == nil {
			response.Body.Close()
			if response.StatusCode != http.StatusNoContent {
				err = fmt.Errorf("status %d", response.StatusCode)
			}
		}
		requestDone <- err
	}()
	select {
	case <-startRequest:
	case err := <-requestDone:
		t.Fatalf("request did not reach handler: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	close(releaseRequest)
	if err := <-requestDone; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	all := events()
	if containsEvent(all, "database") || containsEvent(all, "migrate") {
		t.Fatalf("DB-free executor touched database: %v", all)
	}
	connection, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
	if err == nil {
		connection.Close()
		t.Fatal("listener still open")
	}
	if all[len(all)-1] != "stop" {
		t.Fatalf("events = %v", all)
	}
}

func TestApplicationCanceledBeforeConfiguration(t *testing.T) {
	ops, events, _ := applicationFixture(t, "", "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runApplication(ctx, ApplicationOptions{}, ops); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	if len(events()) != 0 {
		t.Fatalf("side effects: %v", events())
	}
}

type failedApplicationListener struct{ err error }

func (l failedApplicationListener) Accept() (net.Conn, error) { return nil, l.err }
func (failedApplicationListener) Close() error                { return nil }
func (failedApplicationListener) Addr() net.Addr              { return &net.TCPAddr{} }

func TestApplicationServeFailureAndNilDatabase(t *testing.T) {
	ops, events, _ := applicationFixture(t, "", "")
	ops.listen = func(string, string) (net.Listener, error) {
		return failedApplicationListener{errors.New("accept failed")}, nil
	}
	if err := runApplication(context.Background(), ApplicationOptions{}, ops); err == nil || !strings.Contains(err.Error(), "accept failed") {
		t.Fatalf("error = %v", err)
	}
	all := events()
	if !reflect.DeepEqual(all[len(all)-2:], []string{"stop", "close-database"}) {
		t.Fatalf("cleanup = %v", all)
	}
	ops, _, _ = applicationFixture(t, "", "")
	ops.openDatabase = func() (io.Closer, error) { return nil, nil }
	if err := runApplication(context.Background(), ApplicationOptions{}, ops); err == nil {
		t.Fatal("required nil database accepted")
	}
}

func TestCloseApplicationDatabaseRecoversPanic(t *testing.T) {
	err := closeApplicationDatabase(applicationCloser{func() error { panic("close failed") }})
	if err == nil || !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("error = %v", err)
	}
}

func TestApplicationShutdownDeadlineForcesHTTPConnectionsClosed(t *testing.T) {
	ops, _, _ := applicationFixture(t, "", "")
	listening, entered := make(chan string, 1), make(chan struct{})
	listen := ops.listen
	ops.listen = func(network, address string) (net.Listener, error) {
		listener, err := listen(network, address)
		if err == nil {
			listening <- listener.Addr().String()
		}
		return listener, err
	}
	ops.router = func() (http.Handler, error) {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done() }), nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- runApplication(ctx, ApplicationOptions{DisableDatabase: true, ShutdownTimeout: 10 * time.Millisecond}, ops)
	}()
	var address string
	select {
	case address = <-listening:
	case err := <-done:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("listen timeout")
	}
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		client := &http.Client{Timeout: time.Second}
		response, err := client.Get("http://" + address + "/blocked")
		if err == nil {
			response.Body.Close()
		}
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("request timeout")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown error = %v", err)
	}
	<-requestDone
}
