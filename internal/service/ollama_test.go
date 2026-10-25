package service

import (
	"context"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/KEBABSELLER6/embeddings/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testModel = "test-model"

// newTestService starts a fake Ollama server running h and returns a service
// pointed at it. The server is closed when the test ends.
func newTestService(t *testing.T, h http.HandlerFunc, timeout time.Duration) *OllamaService {
	t.Helper()

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	cli := &http.Client{Timeout: timeout}
	return NewOllamaService(cli, config.OllamaConfig{Model: testModel, Endpoint: srv.URL})
}

func respondJSON(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

// blockUntilClientGone never answers. It drains the body first because the
// server only notices a disconnected client once the body has been read.
func blockUntilClientGone(_ http.ResponseWriter, r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)
	select {
	case <-r.Context().Done():
	case <-time.After(5 * time.Second):
	}
}

func TestOllamaServiceEmbed(t *testing.T) {
	t.Run("sends the model and input and returns the embeddings", func(t *testing.T) {
		input := []string{"hello", "world"}

		svc := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
			// assert, not require: this runs on the server's goroutine.
			assert.Equal(t, http.MethodPost, r.Method)

			var got OllamaRequest
			assert.NoError(t, json.UnmarshalRead(r.Body, &got))
			assert.Equal(t, OllamaRequest{Model: testModel, Input: input}, got)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			respondJSON(http.StatusOK, `{"model":"test-model","embeddings":[[0.5,0.25],[1,2]]}`)(w, r)
		}, time.Second)

		got, err := svc.Embed(context.Background(), input)

		require.NoError(t, err)
		assert.Equal(t, &common.EmbeddingResponse{
			Model:      testModel,
			Embeddings: [][]float32{{0.5, 0.25}, {1, 2}},
		}, got)
	})

	errTests := []struct {
		name    string
		handler http.HandlerFunc
		wantIs  error
		wantMsg string
	}{
		{
			name:    "model not found",
			handler: respondJSON(http.StatusNotFound, `{"error":"model \"x\" not found, try pulling it first"}`),
			wantIs:  common.ErrUpstream,
			wantMsg: "not found, try pulling it first",
		},
		{
			name:    "server error",
			handler: respondJSON(http.StatusInternalServerError, `{"error":"boom"}`),
			wantIs:  common.ErrUpstream,
			wantMsg: "status 500",
		},
		{
			name:    "malformed json",
			handler: respondJSON(http.StatusOK, `{"embeddings":`),
			wantIs:  common.ErrInvalidOutput,
		},
		{
			name:    "empty body",
			handler: respondJSON(http.StatusOK, ``),
			wantIs:  common.ErrInvalidOutput,
		},
		{
			name:    "fewer embeddings than inputs",
			handler: respondJSON(http.StatusOK, `{"model":"test-model","embeddings":[[1,2]]}`),
			wantIs:  common.ErrInvalidOutput,
		},
		{
			name:    "more embeddings than inputs",
			handler: respondJSON(http.StatusOK, `{"model":"test-model","embeddings":[[1],[2],[3]]}`),
			wantIs:  common.ErrInvalidOutput,
		},
		{
			name:    "no embeddings field",
			handler: respondJSON(http.StatusOK, `{"model":"test-model"}`),
			wantIs:  common.ErrInvalidOutput,
		},
	}

	for _, tt := range errTests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService(t, tt.handler, time.Second)

			got, err := svc.Embed(context.Background(), []string{"a", "b"})

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantIs)
			if tt.wantMsg != "" {
				assert.ErrorContains(t, err, tt.wantMsg)
			}
			assert.Nil(t, got)
		})
	}

	t.Run("truncates a very large upstream error body", func(t *testing.T) {
		svc := newTestService(t, respondJSON(http.StatusBadGateway, strings.Repeat("x", 10000)), time.Second)

		_, err := svc.Embed(context.Background(), []string{"a"})

		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrUpstream)
		assert.Less(t, len(err.Error()), 2000)
	})

	t.Run("client timeout is an upstream timeout", func(t *testing.T) {
		svc := newTestService(t, blockUntilClientGone, 50*time.Millisecond)

		_, err := svc.Embed(context.Background(), []string{"a"})

		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrUpstream)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("context deadline is an upstream timeout", func(t *testing.T) {
		svc := newTestService(t, blockUntilClientGone, 0)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err := svc.Embed(ctx, []string{"a"})

		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrUpstream)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("cancelled context stops the request", func(t *testing.T) {
		svc := newTestService(t, blockUntilClientGone, 0)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := svc.Embed(ctx, []string{"a"})

		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("cancel mid-request is both upstream and canceled", func(t *testing.T) {
		svc := newTestService(t, blockUntilClientGone, 0)

		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(50*time.Millisecond, cancel)

		_, err := svc.Embed(ctx, []string{"a"})

		// Both match, so HandleError must check Canceled before ErrUpstream.
		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrUpstream)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("unreachable server is an upstream error", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		endpoint := srv.URL
		srv.Close()

		svc := NewOllamaService(&http.Client{Timeout: time.Second}, config.OllamaConfig{Model: testModel, Endpoint: endpoint})

		_, err := svc.Embed(context.Background(), []string{"a"})

		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrUpstream)
	})

	t.Run("invalid endpoint is not reported as an upstream error", func(t *testing.T) {
		svc := NewOllamaService(&http.Client{}, config.OllamaConfig{Model: testModel, Endpoint: "http://bad host/"})

		_, err := svc.Embed(context.Background(), []string{"a"})

		require.Error(t, err)
		assert.NotErrorIs(t, err, common.ErrUpstream)
		assert.ErrorContains(t, err, "internal error")
	})
}
