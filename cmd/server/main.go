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
		"batch_size", conf.BatchSize,
		"grpc_reflection", conf.GRPCReflection,
	)

	embedder, err := NewEmbedder(conf, cli, conf.BatchSize)
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
	ready := handler.NewReadiness()
	router := handler.NewRouter(handler.Handlers{
		Logger:           logger,
		MaxBodySize:      conf.MaxBodySize,
		EmbeddingHandler: handler.NewEmbeddingHandler(embedder, eh, conf.MaxInputs),
		MetricsHandler:   handler.NewMetricsHandler(metr),
		Readiness:        ready,
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
	streams := grpchandler.NewStreamCanceller()
	grpcSrv, grpcHs := grpchandler.NewServer(grpchandler.Handlers{
		Embed:             grpchandler.NewEmbeddingHandler(embedder, grpcEH, conf.MaxInputs),
		Logging:           grpchandler.NewLoggingHandler(logger),
		Correlation:       grpchandler.NewCorrelationHandler(logger),
		MaxBodySize:       int(conf.MaxBodySize),
		MetricsHandler:    grpchandler.NewMetricsHandler(metr),
		Logger:            logger,
		StreamCanceller:   streams,
		ReflectionEnabled: conf.GRPCReflection,
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
			errCh <- fmt.Errorf("grpc server: %w", err)
			return
		}
		if err := grpcSrv.Serve(lis); err != nil {
			errCh <- fmt.Errorf("grpc server: %w", err)
		}
	}()
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			errCh <- fmt.Errorf("server: %w", err)
		}
	}()
	go func() {
		if err := metricsSrv.ListenAndServe(); err != nil {
			errCh <- fmt.Errorf("metrics server: %w", err)
		}
	}()

	var runErr error
	select {
	case runErr = <-errCh:
	case <-ctx.Done():
		// a signal arrived
	}
	stop()

	grpcHs.Shutdown()
	ready.SetDraining()

	logger.Info("shutting down server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), conf.Timeouts.Shutdown)
	defer cancel()

	if runErr == nil && conf.Timeouts.Drain > 0 {
		logger.Info("draining", "delay", conf.Timeouts.Drain)
		time.Sleep(conf.Timeouts.Drain)
	}

	streams.Cancel()

	grpcDone := make(chan struct{})
	go func() { grpcSrv.GracefulStop(); close(grpcDone) }()

	httpErr := srv.Shutdown(shutdownCtx)
	if httpErr != nil {
		srv.Close()
	}
	select {
	case <-grpcDone:
	case <-shutdownCtx.Done():
		grpcSrv.Stop()
		<-grpcDone
	}

	metricsCtx, cancelMetrics := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelMetrics()
	metricsErr := metricsSrv.Shutdown(metricsCtx)
	if metricsErr != nil {
		metricsSrv.Close()
	}

	logger.Info("server stopped")
	return errors.Join(runErr, httpErr, metricsErr)
}
