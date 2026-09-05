package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/kwhitestone/prism-fusion/plugin"
)

// ApplicationOptions describes host infrastructure, never business setup hooks.
// Addons own business configuration, initialization and background workers.
type ApplicationOptions struct {
	ConfigPath string
	// DisableDatabase is explicit for database-free hosts such as executors.
	DisableDatabase bool
	// ShutdownTimeout bounds HTTP draining and cooperative addon cleanup separately.
	ShutdownTimeout time.Duration
}

var ErrApplicationState = errors.New("application has already been started")
var applicationStarted atomic.Bool

// RunApplication runs one application until SIGINT/SIGTERM, returning startup
// and cleanup failures after resources have been released. Callers may then exit.
func RunApplication(options ApplicationOptions) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return RunApplicationContext(ctx, options)
}

// RunApplicationContext owns process-global framework state and can be called
// once per process. A second/concurrent invocation fails before changing globals.
func RunApplicationContext(ctx context.Context, options ApplicationOptions) error {
	if !applicationStarted.CompareAndSwap(false, true) {
		return ErrApplicationState
	}
	return runApplication(ctx, options, defaultApplicationOperations(options))
}

type applicationOperations struct {
	configure    func() error
	openDatabase func() (io.Closer, error)
	migrate      func() error
	router       func() (http.Handler, error)
	address      func() string
	listen       func(string, string) (net.Listener, error)
	lifecycle    *plugin.Lifecycle
}

func shutdownTimeout(options ApplicationOptions) time.Duration {
	if options.ShutdownTimeout > 0 {
		return options.ShutdownTimeout
	}
	return 30 * time.Second
}

func runApplication(ctx context.Context, options ApplicationOptions, operations applicationOperations) (err error) {
	// External cancellation cancels startup immediately. Once serving, workers
	// remain alive while accepted HTTP requests drain, then cancel before Stop.
	workerCtx, cancelWorkers := context.WithCancel(context.WithoutCancel(ctx))
	stopStartupCancellation := context.AfterFunc(ctx, cancelWorkers)
	defer stopStartupCancellation()
	var database io.Closer
	var server *http.Server
	phase := "configuration"
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.Join(err, fmt.Errorf("application %s panicked: %v", phase, recovered))
		}
		if server != nil {
			err = errors.Join(err, shutdownHTTP(server, shutdownTimeout(options)))
		}
		cancelWorkers()
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout(options))
		defer cancel()
		err = errors.Join(err, operations.lifecycle.Stop(cleanupCtx))
		if database != nil {
			err = errors.Join(err, closeApplicationDatabase(database))
		}
	}()
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = operations.configure(); err != nil {
		return err
	}
	if !options.DisableDatabase {
		phase = "database"
		if database, err = operations.openDatabase(); err != nil {
			return err
		}
		if database == nil {
			return errors.New("database is required but unavailable")
		}
	}
	phase = "prepare"
	if err = operations.lifecycle.Prepare(workerCtx); err != nil {
		return err
	}
	if !options.DisableDatabase {
		phase = "migration"
		if err = operations.migrate(); err != nil {
			return err
		}
	}
	phase = "routes"
	handler, err := operations.router()
	if err != nil {
		return err
	}
	phase = "start"
	if err = operations.lifecycle.Start(workerCtx); err != nil {
		return err
	}
	stopStartupCancellation()
	if err = ctx.Err(); err != nil {
		return err
	}
	phase = "listen"
	listener, err := operations.listen("tcp", operations.address())
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer listener.Close()
	server = &http.Server{
		Handler: handler, ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout: 10 * time.Minute, WriteTimeout: 10 * time.Minute,
		IdleTimeout: time.Minute, MaxHeaderBytes: 1 << 20,
		BaseContext: func(net.Listener) context.Context { return workerCtx },
	}
	phase = "serve"
	return serveApplication(ctx, server, listener)
}

func serveApplication(ctx context.Context, server *http.Server, listener net.Listener) error {
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
		return nil
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve application: %w", err)
	}
}

func shutdownHTTP(server *http.Server, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		return errors.Join(fmt.Errorf("drain HTTP: %w", err), server.Close())
	}
	return nil
}

func closeApplicationDatabase(database io.Closer) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("close database panicked: %v", recovered)
		}
	}()
	return database.Close()
}
