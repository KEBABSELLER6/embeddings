package grpchandler

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

type LoggingHandler struct {
	logger *slog.Logger
}

func NewLoggingHandler(logger *slog.Logger) *LoggingHandler {
	return &LoggingHandler{logger: logger}
}

func (h *LoggingHandler) Logging() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		res, err := handler(ctx, req)

		h.logger.InfoContext(ctx, "request",
			"method", info.FullMethod,
			"duration_ms", time.Since(start).Milliseconds(),
			"code", status.Code(err).String(),
		)
		return res, err
	}
}
