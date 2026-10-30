package grpchandler

import (
	"testing"
	"time"

	embeddingsv1 "github.com/KEBABSELLER6/embeddings/gen/embeddings/v1"
	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// openWatch starts a health Watch and waits for its first update, so the
// stream is known to be running on the server before the test goes on.
func openWatch(t *testing.T, ts *testServer) healthpb.Health_WatchClient {
	t.Helper()

	stream, err := healthpb.NewHealthClient(ts.conn).Watch(t.Context(), &healthpb.HealthCheckRequest{})
	require.NoError(t, err)

	resp, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.GetStatus())

	return stream
}

func TestStreamCanceller(t *testing.T) {
	t.Run("a watch still sees status changes through the interceptor", func(t *testing.T) {
		ts := startTestServerFull(t, &fakeEmbeddingService{})
		stream := openWatch(t, ts)

		// What main does first on SIGTERM; watchers must hear about it
		// before their streams are cancelled.
		ts.hs.Shutdown()

		resp, err := stream.Recv()
		require.NoError(t, err)
		assert.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, resp.GetStatus())
	})

	t.Run("cancel ends an open watch", func(t *testing.T) {
		ts := startTestServerFull(t, &fakeEmbeddingService{})
		stream := openWatch(t, ts)

		ts.streams.Cancel()

		_, err := stream.Recv()
		require.Error(t, err)
		assert.Equal(t, codes.Canceled, status.Code(err))
	})

	t.Run("a watch opened after cancel ends right away", func(t *testing.T) {
		ts := startTestServerFull(t, &fakeEmbeddingService{})
		ts.streams.Cancel()

		stream, err := healthpb.NewHealthClient(ts.conn).Watch(t.Context(), &healthpb.HealthCheckRequest{})
		require.NoError(t, err)

		// The server may get the first status out before it sees the
		// cancellation, so read until the stream ends.
		for {
			_, err = stream.Recv()
			if err != nil {
				break
			}
		}
		assert.Equal(t, codes.Canceled, status.Code(err))
	})

	t.Run("unary calls are not affected by cancel", func(t *testing.T) {
		svc := &fakeEmbeddingService{resp: &common.EmbeddingResponse{Model: "m", Embeddings: [][]float32{{1}}}}
		ts := startTestServerFull(t, svc)
		ts.streams.Cancel()

		_, err := embeddingsv1.NewEmbeddingServiceClient(ts.conn).Embed(t.Context(), &embeddingsv1.EmbedRequest{Input: []string{"hi"}})

		require.NoError(t, err)
	})

	t.Run("graceful stop waits on a watch until cancel", func(t *testing.T) {
		ts := startTestServerFull(t, &fakeEmbeddingService{})
		openWatch(t, ts)

		stopped := make(chan struct{})
		go func() { ts.srv.GracefulStop(); close(stopped) }()

		select {
		case <-stopped:
			t.Fatal("GracefulStop returned while a watch was still open")
		case <-time.After(100 * time.Millisecond):
		}

		ts.streams.Cancel()

		select {
		case <-stopped:
		case <-time.After(2 * time.Second):
			t.Fatal("GracefulStop did not return after the watch was cancelled")
		}
	})
}
