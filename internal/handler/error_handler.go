package handler

import (
	"context"
	"encoding/json/v2"
	"errors"
	"log/slog"
	"net/http"

	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/KEBABSELLER6/embeddings/internal/logging"
)

const statusClientClosedRequest = 499

type ErrorHandler struct {
	logger *slog.Logger
}

func NewErrorHandler(logger *slog.Logger) *ErrorHandler {
	return &ErrorHandler{logger: logger}
}

type ErrorResponse struct {
	Message       string `json:"message"`
	CorrelationID string `json:"correlation_id"`
}

func buildError(w http.ResponseWriter, msg string, correlationID string, status int) {
	w.Header().Set("Content-Type", "application/json")
	resp := &ErrorResponse{Message: msg, CorrelationID: correlationID}
	out, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"internal error"}`))
		return
	}
	w.WriteHeader(status)
	w.Write(out)
}

func (h *ErrorHandler) HandleError(w http.ResponseWriter, r *http.Request, err error) {
	correlationID := logging.CorrelationIDFromContext(r.Context())
	if errors.Is(err, context.Canceled) {
		h.logger.WarnContext(r.Context(), "client disconnected", "err", err)
		buildError(w, "client disconnected", correlationID, statusClientClosedRequest)
		return
	}

	if errors.Is(err, common.ErrAuthFailed) {
		h.logger.WarnContext(r.Context(), "unauthenticated", "err", err)
		buildError(w, "authentication failed", correlationID, http.StatusUnauthorized)
		return
	}
	if errors.Is(err, common.ErrMissingAPIKey) {
		h.logger.WarnContext(r.Context(), "unauthenticated", "err", err)
		buildError(w, "api key is missing", correlationID, http.StatusUnauthorized)
		return
	}

	h.logger.ErrorContext(r.Context(), "request failed", "err", err)

	var maxErr *http.MaxBytesError
	switch {
	case errors.As(err, &maxErr):
		buildError(w, "request too large", correlationID, http.StatusRequestEntityTooLarge)
	case errors.Is(err, common.ErrInvalidInput):
		buildError(w, "invalid input", correlationID, http.StatusBadRequest)
	case errors.Is(err, context.DeadlineExceeded):
		buildError(w, "timeout", correlationID, http.StatusGatewayTimeout)
	case errors.Is(err, common.ErrUpstream):
		buildError(w, "embedding backend unavailable", correlationID, http.StatusBadGateway)
	case errors.Is(err, common.ErrInvalidOutput):
		buildError(w, "invalid output", correlationID, http.StatusBadGateway)
	default:
		buildError(w, "internal error", correlationID, http.StatusInternalServerError)
	}
}
