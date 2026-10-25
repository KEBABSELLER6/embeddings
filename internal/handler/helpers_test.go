package handler

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"

	"github.com/KEBABSELLER6/embeddings/internal/logging"
)

const testCorrelationID = "test-correlation-id"

func newRequestWithCorrelationID(method, target string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	return req.WithContext(logging.WithCorrelationID(req.Context(), testCorrelationID))
}

func newTestErrorHandler() *ErrorHandler {
	return NewErrorHandler(slog.New(slog.DiscardHandler))
}
