package service

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/KEBABSELLER6/embeddings/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAPIKey = "test-key"

// newTestOpenAIService starts a fake OpenAI-compatible server running h and
// returns a service pointed at it. The server is closed when the test ends.
func newTestOpenAIService(t *testing.T, h http.HandlerFunc, timeout time.Duration) *OpenAIService {
	t.Helper()

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	cfg := config.OpenAIConfig{APIKey: testAPIKey, Model: testModel, BaseURL: srv.URL + "/v1"}
	return NewOpenAIService(&http.Client{Timeout: timeout}, cfg)
}

// respondAPI is respondJSON with the Content-Type header the OpenAI SDK
// insists on before it will parse a response.
func respondAPI(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

// openAIEntry is one element of the response's data array.
func openAIEntry(index int, vector ...float64) string {
	v, _ := json.Marshal(vector)
	return fmt.Sprintf(`{"object":"embedding","index":%d,"embedding":%s}`, index, v)
}

func openAIBody(model string, entries ...string) string {
	return fmt.Sprintf(
		`{"object":"list","model":%q,"data":[%s],"usage":{"prompt_tokens":0,"total_tokens":0}}`,
		model, strings.Join(entries, ","),
	)
}

func TestOpenAIServiceEmbed(t *testing.T) {
	t.Run("sends the model, input and key and returns the embeddings", func(t *testing.T) {
		input := []string{"hello", "world"}

		svc := newTestOpenAIService(t, func(w http.ResponseWriter, r *http.Request) {
			// assert, not require: this runs on the server's goroutine.
			assert.Equal(t, http.MethodPost, r.Method)
			assert.True(t, strings.HasSuffix(r.URL.Path, "/embeddings"), r.URL.Path)
			assert.Equal(t, "Bearer "+testAPIKey, r.Header.Get("Authorization"))

			var got struct {
				Model          string   `json:"model"`
				Input          []string `json:"input"`
				EncodingFormat string   `json:"encoding_format"`
			}
			assert.NoError(t, json.UnmarshalRead(r.Body, &got))
			assert.Equal(t, testModel, got.Model)
			assert.Equal(t, input, got.Input)
			assert.Equal(t, "float", got.EncodingFormat)

			respondAPI(http.StatusOK, openAIBody("reported-model", openAIEntry(0, 0.5, 0.25), openAIEntry(1, 1, 2)))(w, r)
		}, time.Second)

		got, err := svc.Embed(context.Background(), input)

		require.NoError(t, err)
		assert.Equal(t, &common.EmbeddingResponse{
			Model:      "reported-model",
			Embeddings: [][]float32{{0.5, 0.25}, {1, 2}},
		}, got)
	})

	t.Run("places embeddings by index, not by position", func(t *testing.T) {
		svc := newTestOpenAIService(t, respondAPI(http.StatusOK,
			openAIBody(testModel, openAIEntry(2, 2), openAIEntry(0, 0), openAIEntry(1, 1)),
		), time.Second)

		got, err := svc.Embed(context.Background(), []string{"a", "b", "c"})

		require.NoError(t, err)
		assert.Equal(t, [][]float32{{0}, {1}, {2}}, got.Embeddings)
	})

	errTests := []struct {
		name    string
		handler http.HandlerFunc
		wantIs  error
		notIs   error
		wantMsg string
	}{
		{
			name:    "unauthorized",
			handler: respondAPI(http.StatusUnauthorized, `{"error":{"message":"bad key","type":"invalid_request_error"}}`),
			wantIs:  common.ErrUpstream,
		},
		{
			name:    "rate limited",
			handler: respondAPI(http.StatusTooManyRequests, `{"error":{"message":"slow down","type":"rate_limit_error"}}`),
			wantIs:  common.ErrUpstream,
		},
		{
			name:    "server error",
			handler: respondAPI(http.StatusInternalServerError, `{"error":"boom"}`),
			wantIs:  common.ErrUpstream,
		},
		{
			name:    "malformed json",
			handler: respondAPI(http.StatusOK, `{"data":`),
			wantIs:  common.ErrUpstream,
		},
		{
			name:    "fewer embeddings than inputs",
			handler: respondAPI(http.StatusOK, openAIBody(testModel, openAIEntry(0, 1))),
			wantIs:  common.ErrInvalidOutput,
			notIs:   common.ErrUpstream,
		},
		{
			name:    "more embeddings than inputs",
			handler: respondAPI(http.StatusOK, openAIBody(testModel, openAIEntry(0, 1), openAIEntry(1, 2), openAIEntry(2, 3))),
			wantIs:  common.ErrInvalidOutput,
			notIs:   common.ErrUpstream,
		},
		{
			name:    "no embeddings",
			handler: respondAPI(http.StatusOK, openAIBody(testModel)),
			wantIs:  common.ErrInvalidOutput,
			notIs:   common.ErrUpstream,
		},
		{
			name:    "index past the end",
			handler: respondAPI(http.StatusOK, openAIBody(testModel, openAIEntry(0, 1), openAIEntry(7, 2))),
			wantIs:  common.ErrInvalidOutput,
			notIs:   common.ErrUpstream,
			wantMsg: "bad embedding index 7",
		},
		{
			name:    "negative index",
			handler: respondAPI(http.StatusOK, openAIBody(testModel, openAIEntry(0, 1), openAIEntry(-1, 2))),
			wantIs:  common.ErrInvalidOutput,
			notIs:   common.ErrUpstream,
			wantMsg: "bad embedding index -1",
		},
		{
			name:    "duplicate index",
			handler: respondAPI(http.StatusOK, openAIBody(testModel, openAIEntry(0, 1), openAIEntry(0, 2))),
			wantIs:  common.ErrInvalidOutput,
			notIs:   common.ErrUpstream,
			wantMsg: "bad embedding index 0",
		},
	}

	for _, tt := range errTests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestOpenAIService(t, tt.handler, time.Second)

			got, err := svc.Embed(context.Background(), []string{"a", "b"})

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantIs)
			if tt.notIs != nil {
				assert.NotErrorIs(t, err, tt.notIs)
			}
			if tt.wantMsg != "" {
				assert.ErrorContains(t, err, tt.wantMsg)
			}
			assert.Nil(t, got)
		})
	}

	t.Run("does not retry a failed request", func(t *testing.T) {
		var requests atomic.Int32
		svc := newTestOpenAIService(t, func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			respondAPI(http.StatusInternalServerError, `{"error":"boom"}`)(w, r)
		}, time.Second)

		_, err := svc.Embed(context.Background(), []string{"a"})

		require.Error(t, err)
		assert.EqualValues(t, 1, requests.Load())
	})

	t.Run("client timeout is an upstream timeout", func(t *testing.T) {
		svc := newTestOpenAIService(t, blockUntilClientGone, 50*time.Millisecond)

		_, err := svc.Embed(context.Background(), []string{"a"})

		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrUpstream)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("context deadline is an upstream timeout", func(t *testing.T) {
		svc := newTestOpenAIService(t, blockUntilClientGone, 0)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err := svc.Embed(ctx, []string{"a"})

		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrUpstream)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("cancelled context stops the request", func(t *testing.T) {
		svc := newTestOpenAIService(t, blockUntilClientGone, 0)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := svc.Embed(ctx, []string{"a"})

		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("cancel mid-request is both upstream and canceled", func(t *testing.T) {
		svc := newTestOpenAIService(t, blockUntilClientGone, 0)

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
		baseURL := srv.URL + "/v1"
		srv.Close()

		svc := NewOpenAIService(&http.Client{Timeout: time.Second}, config.OpenAIConfig{APIKey: testAPIKey, Model: testModel, BaseURL: baseURL})

		_, err := svc.Embed(context.Background(), []string{"a"})

		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrUpstream)
	})
}
