package logging

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testCorrelationID = "test-correlation-id"

// newTestLogger wraps a real JSON handler, so WithAttrs and WithGroup behave
// exactly as they do in main.
func newTestLogger(level slog.Level) (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: level})
	return slog.New(NewContextHandler(inner)), &buf
}

// decodeLine parses the single JSON log line in buf.
func decodeLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line), "log output: %q", buf.String())
	return line
}

func TestContextHandler(t *testing.T) {
	ctxWithID := WithCorrelationID(context.Background(), testCorrelationID)

	t.Run("adds correlation id from context", func(t *testing.T) {
		logger, buf := newTestLogger(slog.LevelInfo)

		logger.InfoContext(ctxWithID, "hello")

		line := decodeLine(t, buf)
		assert.Equal(t, "hello", line["msg"])
		assert.Equal(t, testCorrelationID, line["correlation_id"])
	})

	t.Run("omits correlation id when context has none", func(t *testing.T) {
		logger, buf := newTestLogger(slog.LevelInfo)

		logger.InfoContext(context.Background(), "hello")

		assert.NotContains(t, decodeLine(t, buf), "correlation_id")
	})

	t.Run("logging without a context has no correlation id", func(t *testing.T) {
		logger, buf := newTestLogger(slog.LevelInfo)

		// Info passes context.Background(), so there is nothing to read the id from.
		logger.Info("hello")

		assert.NotContains(t, decodeLine(t, buf), "correlation_id")
	})

	t.Run("keeps correlation id after With", func(t *testing.T) {
		logger, buf := newTestLogger(slog.LevelInfo)

		logger.With("k", "v").InfoContext(ctxWithID, "hello")

		line := decodeLine(t, buf)
		assert.Equal(t, "v", line["k"])
		assert.Equal(t, testCorrelationID, line["correlation_id"])
	})

	t.Run("keeps correlation id after WithGroup", func(t *testing.T) {
		logger, buf := newTestLogger(slog.LevelInfo)

		logger.WithGroup("g").InfoContext(ctxWithID, "hello", "a", 1)

		// Record attrs land inside the open group, the added correlation id included.
		line := decodeLine(t, buf)
		require.IsType(t, map[string]any{}, line["g"])
		group := line["g"].(map[string]any)
		assert.Equal(t, float64(1), group["a"])
		assert.Equal(t, testCorrelationID, group["correlation_id"])
	})

	t.Run("respects the inner handler level", func(t *testing.T) {
		logger, buf := newTestLogger(slog.LevelWarn)

		logger.InfoContext(ctxWithID, "dropped")

		assert.Zero(t, buf.Len())
		assert.False(t, logger.Enabled(ctxWithID, slog.LevelInfo))
		assert.True(t, logger.Enabled(ctxWithID, slog.LevelWarn))
	})
}
