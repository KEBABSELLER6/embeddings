package handler

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KEBABSELLER6/embeddings/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	// Wrapped like in main, so the correlation id comes from the request context.
	logger := slog.New(logging.NewContextHandler(capture))
	h := &LoggingHandler{logger: logger}

	req := newRequestWithCorrelationID(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	handler := h.Logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	handler.ServeHTTP(rr, req)

	require.Len(t, capture.records, 1)
	record := capture.records[0]
	assert.Equal(t, "request", record.Message)

	attrs := map[string]slog.Value{}
	record.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value
		return true
	})

	require.Contains(t, attrs, "duration_ms")
	assert.GreaterOrEqual(t, attrs["duration_ms"].Int64(), int64(0))
	delete(attrs, "duration_ms")

	assert.Equal(t, map[string]slog.Value{
		"method":         slog.StringValue(http.MethodGet),
		"path":           slog.StringValue("/healthz"),
		"status":         slog.IntValue(http.StatusTeapot),
		"correlation_id": slog.StringValue(testCorrelationID),
	}, attrs)
}

func TestLoggingMiddlewareUnwrap(t *testing.T) {
	h := NewLoggingHandler(slog.New(slog.DiscardHandler))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	// The recorder implements http.Flusher, standing in for the server's writer.
	rr := httptest.NewRecorder()

	var flushErr error
	handler := h.Logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only works if ResponseController can get past responseWriter via Unwrap.
		flushErr = http.NewResponseController(w).Flush()
	}))
	handler.ServeHTTP(rr, req)

	require.NoError(t, flushErr)
	assert.True(t, rr.Flushed)
}
