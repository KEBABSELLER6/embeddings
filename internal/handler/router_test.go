package handler

import (
	"encoding/json/v2"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"uuid"

	"github.com/KEBABSELLER6/embeddings/internal/auth"
	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/KEBABSELLER6/embeddings/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testMaxBodySize = 64

// newTestServer runs the full router on a real server. A recorder is not
// enough here: only the server's own ResponseWriter reacts to MaxBytesReader
// by closing the connection.
func newTestServer(t *testing.T, svc common.EmbeddingService) *httptest.Server {
	t.Helper()

	srv, _ := newTestServerWithMetrics(t, svc)
	return srv
}

// newTestServerWithMetrics also returns the request metrics, backed by a
// registry private to this test.
func newTestServerWithMetrics(t *testing.T, svc common.EmbeddingService) (*httptest.Server, *metrics.Metrics) {
	t.Helper()

	srv, m, _ := newTestServerFull(t, svc)
	return srv, m
}

// newTestServerFull also returns the readiness state, so a test can start
// draining.
func newTestServerFull(t *testing.T, svc common.EmbeddingService) (*httptest.Server, *metrics.Metrics, *Readiness) {
	t.Helper()

	return newTestServerWithAPIKey(t, svc, "")
}

// newTestServerWithAPIKey requires apiKey on the embed route, like main does
// with API_KEY set. An empty apiKey turns auth off.
func newTestServerWithAPIKey(t *testing.T, svc common.EmbeddingService, apiKey string) (*httptest.Server, *metrics.Metrics, *Readiness) {
	t.Helper()

	m := metrics.New(prometheus.NewRegistry())
	ready := NewReadiness()
	eh := newTestErrorHandler()
	router := NewRouter(Handlers{
		Logger:           slog.New(slog.DiscardHandler),
		MaxBodySize:      testMaxBodySize,
		EmbeddingHandler: NewEmbeddingHandler(svc, eh, testMaxInputs),
		MetricsHandler:   NewMetricsHandler(m),
		Readiness:        ready,
		Auth:             NewAPIKeyAuth(auth.NewAPIKeyService(apiKey), apiKey != "", eh),
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv, m, ready
}

func do(t *testing.T, srv *httptest.Server, method, path, body string, header http.Header) (*http.Response, []byte) {
	t.Helper()

	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	require.NoError(t, err)
	for k, v := range header {
		req.Header[k] = v
	}

	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp, b
}

func decodeErrorResponse(t *testing.T, resp *http.Response, body []byte) ErrorResponse {
	t.Helper()

	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	var got ErrorResponse
	require.NoError(t, json.Unmarshal(body, &got), "body: %q", body)
	return got
}

func assertGeneratedCorrelationID(t *testing.T, resp *http.Response) string {
	t.Helper()

	id := resp.Header.Get("X-Correlation-ID")
	_, err := uuid.Parse(id)
	assert.NoError(t, err, "X-Correlation-ID should be a generated uuid, got %q", id)
	return id
}

func TestRouter(t *testing.T) {
	okResp := &common.EmbeddingResponse{
		Model:      "test-model",
		Embeddings: [][]float32{{0.5, 0.25}},
	}

	t.Run("health check", func(t *testing.T) {
		srv := newTestServer(t, &fakeEmbeddingService{})

		resp, body := do(t, srv, http.MethodGet, "/healthz", "", nil)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"status":"ok"}`, string(body))
		assertGeneratedCorrelationID(t, resp)
	})

	t.Run("readiness check", func(t *testing.T) {
		srv := newTestServer(t, &fakeEmbeddingService{})

		resp, body := do(t, srv, http.MethodGet, "/readyz", "", nil)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"status":"ok"}`, string(body))
		assertGeneratedCorrelationID(t, resp)
	})

	t.Run("draining fails readiness but not liveness", func(t *testing.T) {
		srv, _, ready := newTestServerFull(t, &fakeEmbeddingService{})

		ready.SetDraining()

		resp, body := do(t, srv, http.MethodGet, "/readyz", "", nil)
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
		assert.JSONEq(t, `{"status":"down"}`, string(body))

		resp, body = do(t, srv, http.MethodGet, "/healthz", "", nil)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"status":"ok"}`, string(body))
	})

	t.Run("embed request", func(t *testing.T) {
		srv := newTestServer(t, &fakeEmbeddingService{resp: okResp})

		resp, body := do(t, srv, http.MethodPost, "/api/v1/embed", `{"input":["hi"]}`,
			http.Header{"X-Correlation-Id": {"client-id-1"}})

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
		assert.JSONEq(t, `{"model":"test-model","embeddings":[[0.5,0.25]]}`, string(body))
		assert.Equal(t, "client-id-1", resp.Header.Get("X-Correlation-ID"))
	})

	t.Run("error body carries the same correlation id as the header", func(t *testing.T) {
		srv := newTestServer(t, &fakeEmbeddingService{})

		resp, body := do(t, srv, http.MethodPost, "/api/v1/embed", `{nope`, nil)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		id := assertGeneratedCorrelationID(t, resp)
		assert.Equal(t, ErrorResponse{Message: "invalid input", CorrelationID: id}, decodeErrorResponse(t, resp, body))
	})

	t.Run("oversized body is rejected and the connection is closed", func(t *testing.T) {
		srv := newTestServer(t, &fakeEmbeddingService{resp: okResp})
		oversized := `{"input":["` + strings.Repeat("a", 2*testMaxBodySize) + `"]}`

		resp, body := do(t, srv, http.MethodPost, "/api/v1/embed", oversized, nil)

		assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
		// Only happens when MaxBytesReader gets the server's own ResponseWriter,
		// i.e. when the body limit is the outermost middleware.
		assert.True(t, resp.Close, "expected Connection: close on 413")
		id := assertGeneratedCorrelationID(t, resp)
		assert.Equal(t, ErrorResponse{Message: "request too large", CorrelationID: id}, decodeErrorResponse(t, resp, body))
	})

	t.Run("wrong method on embed", func(t *testing.T) {
		srv := newTestServer(t, &fakeEmbeddingService{})

		resp, _ := do(t, srv, http.MethodGet, "/api/v1/embed", "", nil)

		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		assert.Equal(t, http.MethodPost, resp.Header.Get("Allow"))
		assertGeneratedCorrelationID(t, resp)
	})

	t.Run("unknown path", func(t *testing.T) {
		srv := newTestServer(t, &fakeEmbeddingService{})

		resp, _ := do(t, srv, http.MethodGet, "/nope", "", nil)

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		assertGeneratedCorrelationID(t, resp)
	})
}
