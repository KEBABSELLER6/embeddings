package grpchandler

import (
	"context"
	"runtime/debug"

	embeddingsv1 "github.com/KEBABSELLER6/embeddings/gen/embeddings/v1"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

type Handlers struct {
	Embed       *EmbeddingHandler
	Logging     *LoggingHandler
	MaxBodySize int
	Correlation *CorrelationHandler
}

func NewServer(h Handlers) *grpc.Server {
	r := func(ctx context.Context, p any) (err error) {
		h.Logging.logger.ErrorContext(ctx, "grpc panic", "panic", p, "stack", string(debug.Stack()))
		return status.Error(codes.Internal, "internal error")
	}

	srv := grpc.NewServer(
		grpc.MaxRecvMsgSize(h.MaxBodySize),
		grpc.ChainUnaryInterceptor(
			recovery.UnaryServerInterceptor(recovery.WithRecoveryHandlerContext(r)),
			h.Correlation.CorrelationID(),
			h.Logging.Logging(),
		),
	)

	embeddingsv1.RegisterEmbeddingServiceServer(srv, h.Embed)
	reflection.Register(srv)

	return srv
}
