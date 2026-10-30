package handler

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAPIKey = "s3cret-key"

func TestAPIKeyAuth(t *testing.T) {
	okResp := &common.EmbeddingResponse{Model: "test-model", Embeddings: [][]float32{{1}}}

	tests := []struct {
		name        string
		serverKey   string
		header      http.Header
		wantStatus  int
		wantMessage string
	}{
		{
			name:       "auth disabled lets a request without a key through",
			serverKey:  "",
			wantStatus: http.StatusOK,
		},
		{
			name:       "auth disabled ignores a key that is sent anyway",
			serverKey:  "",
			header:     http.Header{"X-Api-Key": {"anything"}},
			wantStatus: http.StatusOK,
		},
		{
			name:        "missing key is rejected",
			serverKey:   testAPIKey,
			wantStatus:  http.StatusUnauthorized,
			wantMessage: "api key is missing",
		},
		{
			name:        "empty key is rejected as missing",
			serverKey:   testAPIKey,
			header:      http.Header{"X-Api-Key": {""}},
			wantStatus:  http.StatusUnauthorized,
			wantMessage: "api key is missing",
		},
		{
			name:        "wrong key is rejected",
			serverKey:   testAPIKey,
			header:      http.Header{"X-Api-Key": {"wrong"}},
			wantStatus:  http.StatusUnauthorized,
			wantMessage: "authentication failed",
		},
		{
			name:       "correct key is let through",
			serverKey:  testAPIKey,
			header:     http.Header{"X-Api-Key": {testAPIKey}},
			wantStatus: http.StatusOK,
		},
		{
			// Header names are case-insensitive in HTTP.
			name:       "header name in any case",
			serverKey:  testAPIKey,
			header:     http.Header{"x-api-key": {testAPIKey}},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeEmbeddingService{resp: okResp}
			srv, _ := newTestServerWithAPIKey(t, svc, tt.serverKey)

			resp, body := do(t, srv, http.MethodPost, "/api/v1/embed", `{"input":["hi"]}`, tt.header)

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
			if tt.wantStatus == http.StatusOK {
				assert.Equal(t, 1, svc.calls)
				return
			}

			assert.Zero(t, svc.calls, "a rejected request must not reach the embedder")
			id := assertGeneratedCorrelationID(t, resp)
			assert.Equal(t, ErrorResponse{Message: tt.wantMessage, CorrelationID: id}, decodeErrorResponse(t, resp, body))
		})
	}

	t.Run("rejected requests are counted in metrics", func(t *testing.T) {
		srv, m := newTestServerWithAPIKey(t, &fakeEmbeddingService{}, testAPIKey)

		do(t, srv, http.MethodPost, "/api/v1/embed", `{"input":["hi"]}`, nil)

		assert.Equal(t, uint64(1), observationCount(t, m, "http", "POST /api/v1/embed"))
	})
}

func TestNewRouterRequiresAuth(t *testing.T) {
	// Defaulting a missing auth handler to "off" would quietly leave the
	// service open, so it must fail at startup instead.
	require.PanicsWithValue(t, "http: APIKeyAuthHandler is required", func() {
		NewRouter(Handlers{
			Logger:           slog.New(slog.DiscardHandler),
			EmbeddingHandler: NewEmbeddingHandler(&fakeEmbeddingService{}, newTestErrorHandler(), testMaxInputs),
		})
	})
}
