package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/KEBABSELLER6/embeddings/internal/config"
	"github.com/KEBABSELLER6/embeddings/internal/handler"
	"github.com/KEBABSELLER6/embeddings/internal/logging"
)

func main() {
	if err := run(); err != nil {
		slog.Error("application stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	conf, err := config.Configure()
	if err != nil {
		return fmt.Errorf("configure: %w", err)
	}
	cli := &http.Client{Timeout: conf.Timeouts.Upstream}

	var h slog.Handler
	switch conf.Log.Format {
	case config.LogFormatJSON:
		h = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: conf.Log.Level})
	case config.LogFormatText:
		h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: conf.Log.Level})
	default:
		return errors.New("invalid log format")
	}
	logger := slog.New(logging.NewContextHandler(h))
	slog.SetDefault(logger)

	logger.Info("config loaded",
		"port", conf.Port,
		"provider", conf.Provider,
		"log_level", conf.Log.Level,
		"log_format", conf.Log.Format,
		"upstream_timeout", conf.Timeouts.Upstream,
		"shutdown_timeout", conf.Timeouts.Shutdown,
		"max_inputs", conf.MaxInputs,
		"max_body_size", conf.MaxBodySize,
	)

	embedder, err := NewEmbedder(conf, cli)
	if err != nil {
		return fmt.Errorf("create embedder: %w", err)
	}

	eh := handler.NewErrorHandler(logger)
	router := handler.NewRouter(handler.Handlers{
		Logger:           logger,
		MaxBodySize:      conf.MaxBodySize,
		EmbeddingHandler: handler.NewEmbeddingHandler(embedder, eh, conf.MaxInputs),
	})

	addr := fmt.Sprintf(":%s", conf.Port)
	logger.Info("server starting", "addr", addr)

	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      conf.Timeouts.Upstream + 5*time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		// a signal arrived
	}

	stop()

	logger.Info("shutting down server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), conf.Timeouts.Shutdown)
	defer cancel()
	err = srv.Shutdown(shutdownCtx)
	if err != nil {
		srv.Close()
		return fmt.Errorf("shutdown: %w", err)
	}
	logger.Info("server stopped")
	return nil
}
