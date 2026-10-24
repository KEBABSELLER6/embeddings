package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/KEBABSELLER6/embeddings/internal/common"
)

func HandleError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("request failed", "err", err, "correlation_id", common.CorrelationIDFromContext(r.Context()))

	var maxErr *http.MaxBytesError
	switch {
	case errors.As(err, &maxErr):
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
	case errors.Is(err, common.ErrInvalidInput):
		http.Error(w, "invalid input", http.StatusBadRequest)
	case errors.Is(err, context.DeadlineExceeded):
		http.Error(w, "timeout", http.StatusGatewayTimeout)
	case errors.Is(err, common.ErrUpstream):
		http.Error(w, "embedding backend unavailable", http.StatusBadGateway)
	case errors.Is(err, common.ErrInvalidOutput):
		http.Error(w, "invalid output", http.StatusBadGateway)
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
