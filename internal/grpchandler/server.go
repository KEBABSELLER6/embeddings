package grpchandler

import (
	"context"
	"log/slog"
	"runtime/debug"

	embeddingsv1 "github.com/KEBABSELLER6/embeddings/gen/embeddings/v1"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/selector"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

type Handlers struct {
	Embed             *EmbeddingHandler
	Logging           *LoggingHandler
	MaxBodySize       int
	Correlation       *CorrelationHandler
	MetricsHandler    *MetricsHandler
	Logger            *slog.Logger
	StreamCanceller   *StreamCanceller
	ReflectionEnabled bool
	APIKeyAuthHandler *APIKeyAuthHandler
}

func NewServer(h Handlers) (*grpc.Server, *health.Server) {
	if h.APIKeyAuthHandler == nil {
		panic("grpc: APIKeyAuthHandler is required")
	}

	logger := h.Logger
	if logger == nil {
		logger = slog.Default()
	}
	canceller := h.StreamCanceller
	if canceller == nil {
		canceller = NewStreamCanceller()
	}

	r := func(ctx context.Context, p any) (err error) {
		logger.ErrorContext(ctx, "grpc panic", "panic", p, "stack", string(debug.Stack()))
		return status.Error(codes.Internal, "internal error")
	}

	allButHealthZ := func(ctx context.Context, callMeta interceptors.CallMeta) bool {
		return healthpb.Health_ServiceDesc.ServiceName != callMeta.Service
	}

	hs := health.NewServer()
	hs.SetServingStatus(embeddingsv1.EmbeddingService_ServiceDesc.ServiceName, healthpb.HealthCheckResponse_SERVING)
	srv := grpc.NewServer(
		grpc.MaxRecvMsgSize(h.MaxBodySize),
		grpc.ChainUnaryInterceptor(
			h.MetricsHandler.Metrics(),
			h.Correlation.CorrelationID(),
			h.Logging.Logging(),
			selector.UnaryServerInterceptor(auth.UnaryServerInterceptor(h.APIKeyAuthHandler.APIKeyAuth), selector.MatchFunc(allButHealthZ)),
			recovery.UnaryServerInterceptor(recovery.WithRecoveryHandlerContext(r)),
		),
		grpc.ChainStreamInterceptor(
			canceller.Interceptor(),
		),
	)

	healthpb.RegisterHealthServer(srv, hs)
	embeddingsv1.RegisterEmbeddingServiceServer(srv, h.Embed)
	if h.ReflectionEnabled {
		reflection.Register(srv)
	}

	return srv, hs
}
