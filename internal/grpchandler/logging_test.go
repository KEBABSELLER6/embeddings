package grpchandler

import (
	"errors"
	"log/slog"
	"testing"

	embeddingsv1 "github.com/KEBABSELLER6/embeddings/gen/embeddings/v1"
	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

const embedMethod = "/embeddings.v1.EmbeddingService/Embed"

func TestLoggingInterceptor(t *testing.T) {
	tests := []struct {
		name     string
		svc      *fakeEmbeddingService
		input    []string
		wantCode string
	}{
		{
			name:     "successful request",
			svc:      &fakeEmbeddingService{resp: &common.EmbeddingResponse{Model: "m", Embeddings: [][]float32{{1}}}},
			input:    []string{"hi"},
			wantCode: "OK",
		},
		{
			name:     "invalid input",
			svc:      &fakeEmbeddingService{},
			input:    nil,
			wantCode: "InvalidArgument",
		},
		{
			name:     "upstream failure",
			svc:      &fakeEmbeddingService{err: common.ErrUpstream},
			input:    []string{"hi"},
			wantCode: "Unavailable",
		},
		{
			name:     "unknown error",
			svc:      &fakeEmbeddingService{err: errors.New("boom")},
			input:    []string{"hi"},
			wantCode: "Internal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, capture := startTestServer(t, tt.svc)

			ctx := metadata.AppendToOutgoingContext(t.Context(), "x-correlation-id", "test-correlation-id")
			_, _ = client.Embed(ctx, &embeddingsv1.EmbedRequest{Input: tt.input})

			records := capture.byMessage("request")
			require.Len(t, records, 1)
			attrs := recordAttrs(records[0])

			require.Contains(t, attrs, "duration_ms")
			assert.GreaterOrEqual(t, attrs["duration_ms"].Int64(), int64(0))
			delete(attrs, "duration_ms")

			assert.Equal(t, map[string]slog.Value{
				"method":         slog.StringValue(embedMethod),
				"code":           slog.StringValue(tt.wantCode),
				"correlation_id": slog.StringValue("test-correlation-id"),
			}, attrs)
		})
	}
}

// Failures are logged once by the error handler and once by the interceptor,
// both with the same correlation id, so a log search by id finds the cause.
func TestErrorHandlerLogsWithCorrelationID(t *testing.T) {
	client, capture := startTestServer(t, &fakeEmbeddingService{err: common.ErrUpstream})

	ctx := metadata.AppendToOutgoingContext(t.Context(), "x-correlation-id", "test-correlation-id")
	_, err := client.Embed(ctx, &embeddingsv1.EmbedRequest{Input: []string{"hi"}})
	require.Error(t, err)

	records := capture.byMessage("grpc error")
	require.Len(t, records, 1)
	attrs := recordAttrs(records[0])
	assert.Equal(t, "test-correlation-id", attrs["correlation_id"].String())
	assert.Contains(t, attrs["err"].String(), common.ErrUpstream.Error())
}
