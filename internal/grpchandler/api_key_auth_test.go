package grpchandler

import (
	"log/slog"
	"testing"

	embeddingsv1 "github.com/KEBABSELLER6/embeddings/gen/embeddings/v1"
	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const testAPIKey = "s3cret-key"

func TestAPIKeyAuth(t *testing.T) {
	okResp := &common.EmbeddingResponse{Model: "test-model", Embeddings: [][]float32{{1}}}

	tests := []struct {
		name        string
		serverKey   string
		md          metadata.MD
		wantCode    codes.Code
		wantMessage string
	}{
		{
			name:      "auth disabled lets a call without a key through",
			serverKey: "",
			wantCode:  codes.OK,
		},
		{
			name:      "auth disabled ignores a key that is sent anyway",
			serverKey: "",
			md:        metadata.Pairs("x-api-key", "anything"),
			wantCode:  codes.OK,
		},
		{
			name:        "missing key is rejected",
			serverKey:   testAPIKey,
			wantCode:    codes.Unauthenticated,
			wantMessage: "missing authentication",
		},
		{
			name:        "wrong key is rejected",
			serverKey:   testAPIKey,
			md:          metadata.Pairs("x-api-key", "wrong"),
			wantCode:    codes.Unauthenticated,
			wantMessage: "authentication failed",
		},
		{
			name:        "empty key is rejected",
			serverKey:   testAPIKey,
			md:          metadata.Pairs("x-api-key", ""),
			wantCode:    codes.Unauthenticated,
			wantMessage: "authentication failed",
		},
		{
			name:      "correct key is let through",
			serverKey: testAPIKey,
			md:        metadata.Pairs("x-api-key", testAPIKey),
			wantCode:  codes.OK,
		},
		{
			// metadata.Pairs lowercases keys, as gRPC requires.
			name:      "key name in any case",
			serverKey: testAPIKey,
			md:        metadata.Pairs("X-API-Key", testAPIKey),
			wantCode:  codes.OK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeEmbeddingService{resp: okResp}
			ts := startTestServerWithAPIKey(t, svc, tt.serverKey)
			client := embeddingsv1.NewEmbeddingServiceClient(ts.conn)

			ctx := t.Context()
			if tt.md != nil {
				ctx = metadata.NewOutgoingContext(ctx, tt.md)
			}
			_, err := client.Embed(ctx, &embeddingsv1.EmbedRequest{Input: []string{"hi"}})

			assert.Equal(t, tt.wantCode, status.Code(err))
			if tt.wantCode == codes.OK {
				assert.Equal(t, 1, svc.calls)
				return
			}

			assert.Zero(t, svc.calls, "a rejected call must not reach the embedder")
			assert.Equal(t, tt.wantMessage, status.Convert(err).Message())
		})
	}

	t.Run("health stays open when auth is on", func(t *testing.T) {
		ts := startTestServerWithAPIKey(t, &fakeEmbeddingService{}, testAPIKey)

		resp, err := healthpb.NewHealthClient(ts.conn).Check(t.Context(), &healthpb.HealthCheckRequest{})

		require.NoError(t, err)
		assert.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.GetStatus())
	})

	t.Run("rejected calls are counted and logged as unauthenticated", func(t *testing.T) {
		ts := startTestServerWithAPIKey(t, &fakeEmbeddingService{}, testAPIKey)
		client := embeddingsv1.NewEmbeddingServiceClient(ts.conn)

		_, err := client.Embed(t.Context(), &embeddingsv1.EmbedRequest{Input: []string{"hi"}})
		require.Error(t, err)

		method := embeddingsv1.EmbeddingService_Embed_FullMethodName
		assert.Equal(t, float64(1), testutil.ToFloat64(ts.metrics.Requests.WithLabelValues("grpc", method, "Unauthenticated")))

		// Anyone can send a bad key; that must not flood the error log.
		records := ts.capture.byMessage("unauthenticated")
		require.Len(t, records, 1)
		assert.Equal(t, slog.LevelWarn, records[0].Level)
		assert.Empty(t, ts.capture.byMessage("grpc error"))
	})
}

func TestNewServerRequiresAuth(t *testing.T) {
	// Defaulting a missing auth handler to "off" would quietly leave the
	// service open, so it must fail at startup instead.
	require.PanicsWithValue(t, "grpc: APIKeyAuthHandler is required", func() {
		NewServer(Handlers{
			Embed:          NewEmbeddingHandler(&fakeEmbeddingService{}, NewErrorHandler(nil), testMaxInputs),
			Logging:        NewLoggingHandler(nil),
			Correlation:    NewCorrelationHandler(nil),
			MetricsHandler: NewMetricsHandler(nil),
		})
	})
}
