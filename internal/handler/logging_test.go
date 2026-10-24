package handler

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/stretchr/testify/assert"
)

type captureHandler struct {
	records []slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler         { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler              { return h }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}

func TestLoggingMiddleware(t *testing.T) {
	capture := &captureHandler{}
	logger := slog.New(capture)
	h := &LoggingHandler{slog: logger}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	ctx := context.WithValue(req.Context(), common.CorrelationKey, "test-correlation-id")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler := h.Logging(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler.ServeHTTP(rr, req)

	assert.Len(t, capture.records, 1)
	record := capture.records[0]
	assert.Equal(t, "request", record.Message)

	record.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "method":
			assert.Equal(t, "GET", a.Value.String())
		case "path":
			assert.Equal(t, "/healthz", a.Value.String())
		case "status":
			assert.Equal(t, int64(200), a.Value.Int64())
		case "duration_ms":
			assert.GreaterOrEqual(t, a.Value.Int64(), int64(0))
		case "correlation_id":
			assert.NotEmpty(t, a.Value.String())
		}
		return true
	})
}
