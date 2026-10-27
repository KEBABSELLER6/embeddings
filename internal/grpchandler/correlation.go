package grpchandler

import (
	"context"
	"log/slog"
	"uuid"

	"github.com/KEBABSELLER6/embeddings/internal/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type CorrelationHandler struct {
	logger *slog.Logger
}

func NewCorrelationHandler(logger *slog.Logger) *CorrelationHandler {
	return &CorrelationHandler{logger: logger}
}

func (h *CorrelationHandler) CorrelationID() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		var id string
		if ok {
			if values := md.Get("x-correlation-id"); len(values) > 0 {
				id = values[0]
			}
		}
		if !logging.ValidCorrelationID(id) {
			id = uuid.NewV7().String()
		}

		ctx = logging.WithCorrelationID(ctx, id)
		err := grpc.SetHeader(ctx, metadata.Pairs("x-correlation-id", id))
		if err != nil {
			h.logger.DebugContext(ctx, "failed to set correlation header", "err", err)
		}
		return handler(ctx, req)
	}
}
