package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/KEBABSELLER6/embeddings/internal/config"
	"github.com/KEBABSELLER6/embeddings/internal/grpchandler"
	"github.com/KEBABSELLER6/embeddings/internal/handler"
	"github.com/KEBABSELLER6/embeddings/internal/logging"
	"github.com/KEBABSELLER6/embeddings/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
		"grpc_port", conf.GRPCPort,
		"metrics_port", conf.MetricsPort,
	)

	embedder, err := NewEmbedder(conf, cli)
	if err != nil {
		return fmt.Errorf("create embedder: %w", err)
	}
	reg := prometheus.NewRegistry()
	metr := metrics.New(reg)
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	eh := handler.NewErrorHandler(logger)
	router := handler.NewRouter(handler.Handlers{
		Logger:           logger,
		MaxBodySize:      conf.MaxBodySize,
		EmbeddingHandler: handler.NewEmbeddingHandler(embedder, eh, conf.MaxInputs),
		MetricsHandler:   handler.NewMetricsHandler(metr),
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

	grpcEH := grpchandler.NewErrorHandler(logger)
	grpcSrv, grpcHs := grpchandler.NewServer(grpchandler.Handlers{
		Embed:          grpchandler.NewEmbeddingHandler(embedder, grpcEH, conf.MaxInputs),
		Logging:        grpchandler.NewLoggingHandler(logger),
		Correlation:    grpchandler.NewCorrelationHandler(logger),
		MaxBodySize:    int(conf.MaxBodySize),
		MetricsHandler: grpchandler.NewMetricsHandler(metr),
	})
	grpcAddr := fmt.Sprintf(":%s", conf.GRPCPort)
	logger.Info("grpc server starting", "addr", grpcAddr)

	metricsAddr := fmt.Sprintf(":%s", conf.MetricsPort)
	metricsMux := http.NewServeMux()
	metricsMux.Handle("GET /metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	metricsSrv := &http.Server{
		Addr:              metricsAddr,
		Handler:           metricsMux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	logger.Info("metrics server starting", "addr", metricsAddr)

	errCh := make(chan error, 3)
	go func() {
		lis, err := net.Listen("tcp", grpcAddr)
		if err != nil {
			errCh <- err
			return
		}
		errCh <- grpcSrv.Serve(lis)
	}()
	go func() { errCh <- srv.ListenAndServe() }()
	go func() { errCh <- metricsSrv.ListenAndServe() }()

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
	grpcHs.Shutdown()
	grpcCh := make(chan struct{})
	go func() { grpcSrv.GracefulStop(); close(grpcCh) }()
	metricsErr := make(chan error, 1)
	go func() { metricsErr <- metricsSrv.Shutdown(shutdownCtx) }()
	err = srv.Shutdown(shutdownCtx)
	select {
	case <-grpcCh:
	case <-shutdownCtx.Done():
		grpcSrv.Stop()
	}
	if mErr := <-metricsErr; mErr != nil {
		metricsSrv.Close()
		err = errors.Join(err, mErr)
	}

	if err != nil {
		srv.Close()
		return fmt.Errorf("shutdown server: %w", err)
	}
	logger.Info("server stopped")
	return nil
}
