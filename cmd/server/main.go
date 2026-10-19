// Command server wires the in-memory adapters into the ride service and serves the HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Swapped out by tests so main itself can run under the race detector and the coverage gate.
var (
	exit             = os.Exit
	stderr io.Writer = os.Stderr
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		exit(1)
	}
}

func run(ctx context.Context, getenv func(string) string, out io.Writer) error {
	cfg, err := loadConfig(getenv)
	if err != nil {
		return err
	}
	handler, err := newHandler(cfg, out)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "listening on http://localhost:%d\n", ln.Addr().(*net.TCPAddr).Port)

	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	shutdown := make(chan error, 1)
	go func() {
		<-ctx.Done()
		stopCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		shutdown <- srv.Shutdown(stopCtx)
	}()

	err = srv.Serve(ln)
	cancel()
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return errors.Join(err, <-shutdown)
}
