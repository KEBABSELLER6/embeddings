package grpchandler

import (
	"context"
	"errors"
	"log/slog"

	"github.com/KEBABSELLER6/embeddings/internal/common"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ErrorHandler struct {
	logger *slog.Logger
}

func NewErrorHandler(logger *slog.Logger) *ErrorHandler {
	return &ErrorHandler{logger: logger}
}

func (h *ErrorHandler) HandleError(ctx context.Context, err error) error {
	if errors.Is(err, context.Canceled) {
		h.logger.WarnContext(ctx, "client disconnected", "err", err)
		return status.Error(codes.Canceled, "client disconnected")
	}

	if errors.Is(err, common.ErrAuthFailed) {
		h.logger.WarnContext(ctx, "unauthenticated", "err", err)
		return status.Error(codes.Unauthenticated, "authentication failed")
	}
	if errors.Is(err, common.ErrMissingAPIKey) {
		h.logger.WarnContext(ctx, "unauthenticated", "err", err)
		return status.Error(codes.Unauthenticated, "missing authentication")
	}

	h.logger.ErrorContext(ctx, "grpc error", "err", err)

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "timeout")
	case errors.Is(err, common.ErrUpstream):
		return status.Error(codes.Unavailable, "embedding backend unavailable")
	case errors.Is(err, common.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, "invalid input")
	case errors.Is(err, common.ErrInvalidOutput):
		return status.Error(codes.Unavailable, "invalid output")
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
