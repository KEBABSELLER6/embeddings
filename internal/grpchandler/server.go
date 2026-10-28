package grpchandler

import (
	"context"
	"runtime/debug"

	embeddingsv1 "github.com/KEBABSELLER6/embeddings/gen/embeddings/v1"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

type Handlers struct {
	Embed          *EmbeddingHandler
	Logging        *LoggingHandler
	MaxBodySize    int
	Correlation    *CorrelationHandler
	MetricsHandler *MetricsHandler
}

func NewServer(h Handlers) (*grpc.Server, *health.Server) {
	r := func(ctx context.Context, p any) (err error) {
		h.Logging.logger.ErrorContext(ctx, "grpc panic", "panic", p, "stack", string(debug.Stack()))
		return status.Error(codes.Internal, "internal error")
	}

	hs := health.NewServer()
	hs.SetServingStatus("embeddings.v1.EmbeddingService", healthpb.HealthCheckResponse_SERVING)
	srv := grpc.NewServer(
		grpc.MaxRecvMsgSize(h.MaxBodySize),
		grpc.ChainUnaryInterceptor(
			h.MetricsHandler.Metrics(),
			recovery.UnaryServerInterceptor(recovery.WithRecoveryHandlerContext(r)),
			h.Correlation.CorrelationID(),
			h.Logging.Logging(),
		),
	)

	healthpb.RegisterHealthServer(srv, hs)
	embeddingsv1.RegisterEmbeddingServiceServer(srv, h.Embed)
	reflection.Register(srv)

	return srv, hs
}
