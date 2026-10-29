package grpchandler

import (
	"context"
	"errors"
	"testing"

	embeddingsv1 "github.com/KEBABSELLER6/embeddings/gen/embeddings/v1"
	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestMetricsCountsRequests(t *testing.T) {
	okResp := &common.EmbeddingResponse{Model: "m", Embeddings: [][]float32{{1}}}

	tests := []struct {
		name       string
		svc        *fakeEmbeddingService
		input      []string
		wantStatus string
	}{
		{
			name:       "successful request",
			svc:        &fakeEmbeddingService{resp: okResp},
			input:      []string{"hi"},
			wantStatus: "OK",
		},
		{
			name:       "invalid input",
			svc:        &fakeEmbeddingService{},
			input:      nil,
			wantStatus: "InvalidArgument",
		},
		{
			name:       "upstream failure",
			svc:        &fakeEmbeddingService{err: common.ErrUpstream},
			input:      []string{"hi"},
			wantStatus: "Unavailable",
		},
		{
			name:       "deadline exceeded",
			svc:        &fakeEmbeddingService{err: context.DeadlineExceeded},
			input:      []string{"hi"},
			wantStatus: "DeadlineExceeded",
		},
		{
			name:       "unknown error",
			svc:        &fakeEmbeddingService{err: errors.New("boom")},
			input:      []string{"hi"},
			wantStatus: "Internal",
		},
		{
			// Metrics sit outside recovery, so a recovered panic is still counted.
			name:       "recovered panic",
			svc:        &fakeEmbeddingService{panicWith: "boom"},
			input:      []string{"hi"},
			wantStatus: "Internal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := startTestServerFull(t, tt.svc)
			client := embeddingsv1.NewEmbeddingServiceClient(ts.conn)

			_, _ = client.Embed(t.Context(), &embeddingsv1.EmbedRequest{Input: tt.input})

			got := testutil.ToFloat64(ts.metrics.Requests.WithLabelValues("grpc", embedMethod, tt.wantStatus))
			assert.Equal(t, float64(1), got)
			assert.Equal(t, 1, testutil.CollectAndCount(ts.metrics.Requests), "exactly one series should exist")

			assert.Equal(t, uint64(1), observationCount(t, ts.metrics, "grpc", embedMethod))
			assert.Equal(t, 1, testutil.CollectAndCount(ts.metrics.Duration), "exactly one series should exist")
		})
	}
}

func TestMetricsAccumulates(t *testing.T) {
	ts := startTestServerFull(t, &fakeEmbeddingService{resp: &common.EmbeddingResponse{Model: "m", Embeddings: [][]float32{{1}}}})
	client := embeddingsv1.NewEmbeddingServiceClient(ts.conn)

	for range 3 {
		_, err := client.Embed(t.Context(), &embeddingsv1.EmbedRequest{Input: []string{"hi"}})
		require.NoError(t, err)
	}

	assert.Equal(t, float64(3), testutil.ToFloat64(ts.metrics.Requests.WithLabelValues("grpc", embedMethod, "OK")))
	assert.Equal(t, uint64(3), observationCount(t, ts.metrics, "grpc", embedMethod))
}

// Probes call Check every few seconds and would drown out real traffic.
func TestMetricsSkipsHealthCheck(t *testing.T) {
	okResp := &common.EmbeddingResponse{Model: "m", Embeddings: [][]float32{{1}}}
	ts := startTestServerFull(t, &fakeEmbeddingService{resp: okResp})

	health := healthpb.NewHealthClient(ts.conn)
	for range 3 {
		resp, err := health.Check(t.Context(), &healthpb.HealthCheckRequest{})
		require.NoError(t, err)
		assert.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.GetStatus())
	}

	assert.Zero(t, testutil.CollectAndCount(ts.metrics.Requests))
	assert.Zero(t, testutil.CollectAndCount(ts.metrics.Duration))

	// Other methods are still counted, and only they.
	client := embeddingsv1.NewEmbeddingServiceClient(ts.conn)
	_, err := client.Embed(t.Context(), &embeddingsv1.EmbedRequest{Input: []string{"hi"}})
	require.NoError(t, err)

	assert.Equal(t, 1, testutil.CollectAndCount(ts.metrics.Requests))
	assert.Equal(t, float64(1), testutil.ToFloat64(ts.metrics.Requests.WithLabelValues("grpc", embedMethod, "OK")))
}
