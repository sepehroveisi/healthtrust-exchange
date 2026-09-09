package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/rc2runtime"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := rc2runtime.ConfigFromEnvironment()
	if err != nil {
		logger.Error("rc2_demo_api_configuration_failed", "error", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	runtime, err := rc2runtime.New(ctx, cfg)
	if err != nil {
		logger.Error("rc2_demo_api_initialization_failed", "error", err)
		os.Exit(1)
	}
	defer runtime.Close()
	server := &http.Server{Addr: cfg.ListenAddress, Handler: runtime.Handler, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		_ = server.Shutdown(shutdown)
	}()
	logger.Info("rc2_demo_api_started", "address", cfg.ListenAddress, "bootstrap_enabled", cfg.BootstrapEnabled)
	if err = server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("rc2_demo_api_failed", "error", err)
		os.Exit(1)
	}
}
