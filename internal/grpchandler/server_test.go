package grpchandler

import (
	"strings"
	"testing"

	embeddingsv1 "github.com/KEBABSELLER6/embeddings/gen/embeddings/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestServerRecoversFromPanic(t *testing.T) {
	svc := &fakeEmbeddingService{panicWith: "secret panic detail"}
	client, capture := startTestServer(t, svc)

	_, err := client.Embed(t.Context(), &embeddingsv1.EmbedRequest{Input: []string{"hi"}})

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Equal(t, "internal error", st.Message())
	assert.NotContains(t, err.Error(), "secret panic detail", "panic text must not reach the client")
	assert.NotContains(t, err.Error(), "goroutine", "stack trace must not reach the client")

	records := capture.byMessage("grpc panic")
	require.Len(t, records, 1)
	attrs := recordAttrs(records[0])
	assert.Equal(t, "secret panic detail", attrs["panic"].String())
	assert.NotEmpty(t, attrs["stack"].String())
}

// A recovered panic must not take the server down for later requests.
func TestServerKeepsServingAfterPanic(t *testing.T) {
	svc := &fakeEmbeddingService{panicWith: "boom"}
	client, _ := startTestServer(t, svc)

	for range 2 {
		_, err := client.Embed(t.Context(), &embeddingsv1.EmbedRequest{Input: []string{"hi"}})
		require.Error(t, err)
		assert.Equal(t, codes.Internal, status.Code(err))
	}
	assert.Equal(t, 2, svc.calls)
}

func TestServerRejectsOversizedRequest(t *testing.T) {
	svc := &fakeEmbeddingService{}
	client, _ := startTestServer(t, svc)

	big := strings.Repeat("x", testMaxBodySize+1)
	_, err := client.Embed(t.Context(), &embeddingsv1.EmbedRequest{Input: []string{big}})

	require.Error(t, err)
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	assert.Zero(t, svc.calls)
}

func TestServerRegistersReflection(t *testing.T) {
	srv := NewServer(Handlers{
		Embed:       NewEmbeddingHandler(&fakeEmbeddingService{}, NewErrorHandler(nil), testMaxInputs),
		Logging:     NewLoggingHandler(nil),
		Correlation: NewCorrelationHandler(nil),
		MaxBodySize: testMaxBodySize,
	})

	info := srv.GetServiceInfo()
	assert.Contains(t, info, "embeddings.v1.EmbeddingService")
	assert.Contains(t, info, "grpc.reflection.v1.ServerReflection")
}
