package grpchandler

import (
	"context"
	"time"

	"github.com/KEBABSELLER6/embeddings/internal/metrics"
	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

type MetricsHandler struct {
	m *metrics.Metrics
}

func NewMetricsHandler(m *metrics.Metrics) *MetricsHandler {
	return &MetricsHandler{m: m}
}

func (h *MetricsHandler) Metrics() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if info.FullMethod == healthpb.Health_Check_FullMethodName {
			return handler(ctx, req)
		}
		start := time.Now()
		res, err := handler(ctx, req)
		h.m.Duration.WithLabelValues("grpc", info.FullMethod).Observe(time.Since(start).Seconds())
		h.m.Requests.WithLabelValues("grpc", info.FullMethod, status.Code(err).String()).Inc()
		return res, err
	}
}
