package grpchandler

import (
	"strings"
	"testing"

	"uuid"

	embeddingsv1 "github.com/KEBABSELLER6/embeddings/gen/embeddings/v1"
	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/KEBABSELLER6/embeddings/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestCorrelationID(t *testing.T) {
	tests := []struct {
		name           string
		incomingHeader string
		wantGenerated  bool
	}{
		{
			name:           "uses existing x-correlation-id header",
			incomingHeader: "test-correlation-id",
			wantGenerated:  false,
		},
		{
			name:           "keeps id with every allowed character class",
			incomingHeader: "abc-XYZ_123.x",
			wantGenerated:  false,
		},
		{
			name:           "keeps id exactly at the length limit",
			incomingHeader: strings.Repeat("a", logging.MaxCorrelationIDLen),
			wantGenerated:  false,
		},
		{
			name:           "generates id when header is absent",
			incomingHeader: "",
			wantGenerated:  true,
		},
		{
			name:           "generates id when header is one over the length limit",
			incomingHeader: strings.Repeat("a", logging.MaxCorrelationIDLen+1),
			wantGenerated:  true,
		},
		{
			name:           "generates id when header contains a space",
			incomingHeader: "has space",
			wantGenerated:  true,
		},
		{
			// The grpc client refuses to send line breaks or non-ascii bytes in
			// header values, so only printable-but-disallowed characters can
			// reach the server.
			name:           "generates id when header contains a disallowed character",
			incomingHeader: "semi;colon",
			wantGenerated:  true,
		},
		{
			name:           "generates id when header contains a slash",
			incomingHeader: "path/like",
			wantGenerated:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeEmbeddingService{resp: &common.EmbeddingResponse{Model: "m", Embeddings: [][]float32{{1}}}}
			client, _ := startTestServer(t, svc)

			ctx := t.Context()
			if tt.incomingHeader != "" {
				ctx = metadata.AppendToOutgoingContext(ctx, "x-correlation-id", tt.incomingHeader)
			}

			var header metadata.MD
			_, err := client.Embed(ctx, &embeddingsv1.EmbedRequest{Input: []string{"hi"}}, grpc.Header(&header))
			require.NoError(t, err)

			got := header.Get("x-correlation-id")
			require.Len(t, got, 1)
			assert.NotEmpty(t, svc.gotCorrelator)
			assert.Equal(t, svc.gotCorrelator, got[0], "header should match the id seen by the handler")

			if tt.wantGenerated {
				assert.NotEqual(t, tt.incomingHeader, got[0])
				_, err := uuid.Parse(got[0])
				require.NoError(t, err, "generated id should be a uuid")
			} else {
				assert.Equal(t, tt.incomingHeader, got[0])
			}
		})
	}
}

// The id must also be returned when the call fails, so clients can quote it.
func TestCorrelationIDOnError(t *testing.T) {
	client, _ := startTestServer(t, &fakeEmbeddingService{})

	var header metadata.MD
	_, err := client.Embed(t.Context(), &embeddingsv1.EmbedRequest{}, grpc.Header(&header))
	require.Error(t, err)

	got := header.Get("x-correlation-id")
	require.Len(t, got, 1)
	_, parseErr := uuid.Parse(got[0])
	require.NoError(t, parseErr)
}
