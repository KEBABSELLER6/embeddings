package handler

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/KEBABSELLER6/embeddings/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricsCountsRequests(t *testing.T) {
	okResp := &common.EmbeddingResponse{Model: "m", Embeddings: [][]float32{{1}}}

	tests := []struct {
		name       string
		svc        *fakeEmbeddingService
		method     string
		path       string
		body       string
		wantStatus int
		wantRoute  string
	}{
		{
			name:       "embed request",
			svc:        &fakeEmbeddingService{resp: okResp},
			method:     http.MethodPost,
			path:       "/api/v1/embed",
			body:       `{"input":["hello"]}`,
			wantStatus: http.StatusOK,
			wantRoute:  "POST /api/v1/embed",
		},
		{
			name:       "invalid json",
			svc:        &fakeEmbeddingService{},
			method:     http.MethodPost,
			path:       "/api/v1/embed",
			body:       `{`,
			wantStatus: http.StatusBadRequest,
			wantRoute:  "POST /api/v1/embed",
		},
		{
			name:       "upstream failure",
			svc:        &fakeEmbeddingService{err: common.ErrUpstream},
			method:     http.MethodPost,
			path:       "/api/v1/embed",
			body:       `{"input":["hello"]}`,
			wantStatus: http.StatusBadGateway,
			wantRoute:  "POST /api/v1/embed",
		},
		{
			name:       "body too large",
			svc:        &fakeEmbeddingService{},
			method:     http.MethodPost,
			path:       "/api/v1/embed",
			body:       `{"input":["` + strings.Repeat("x", testMaxBodySize) + `"]}`,
			wantStatus: http.StatusRequestEntityTooLarge,
			wantRoute:  "POST /api/v1/embed",
		},
		{
			name:       "unknown path is not a label value",
			svc:        &fakeEmbeddingService{},
			method:     http.MethodGet,
			path:       "/some/user/chosen/path",
			wantStatus: http.StatusNotFound,
			wantRoute:  "unmatched",
		},
		{
			name:       "wrong method",
			svc:        &fakeEmbeddingService{},
			method:     http.MethodPut,
			path:       "/healthz",
			wantStatus: http.StatusMethodNotAllowed,
			wantRoute:  "unmatched",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, m := newTestServerWithMetrics(t, tt.svc)

			resp, _ := do(t, srv, tt.method, tt.path, tt.body, nil)
			assert.Equal(t, tt.wantStatus, resp.StatusCode)

			got := testutil.ToFloat64(m.Requests.WithLabelValues("http", tt.wantRoute, strconv.Itoa(tt.wantStatus)))
			assert.Equal(t, float64(1), got)
			assert.Equal(t, 1, testutil.CollectAndCount(m.Requests), "exactly one series should exist")

			assert.Equal(t, uint64(1), observationCount(t, m, "http", tt.wantRoute))
			assert.Equal(t, 1, testutil.CollectAndCount(m.Duration), "exactly one series should exist")
		})
	}
}

func TestMetricsAccumulates(t *testing.T) {
	okResp := &common.EmbeddingResponse{Model: "m", Embeddings: [][]float32{{1}}}
	srv, m := newTestServerWithMetrics(t, &fakeEmbeddingService{resp: okResp})

	for range 3 {
		do(t, srv, http.MethodPost, "/api/v1/embed", `{"input":["hello"]}`, nil)
	}

	assert.Equal(t, float64(3), testutil.ToFloat64(m.Requests.WithLabelValues("http", "POST /api/v1/embed", "200")))
	assert.Equal(t, uint64(3), observationCount(t, m, "http", "POST /api/v1/embed"))
}

// Probes hit /healthz every few seconds and would drown out real traffic.
func TestMetricsSkipsHealthRequests(t *testing.T) {
	srv, m := newTestServerWithMetrics(t, &fakeEmbeddingService{})

	resp, _ := do(t, srv, http.MethodGet, "/healthz", "", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	assert.Zero(t, testutil.CollectAndCount(m.Requests))
	assert.Zero(t, testutil.CollectAndCount(m.Duration))

	// Only the exact health route is skipped; a wrong method is still counted.
	resp, _ = do(t, srv, http.MethodPut, "/healthz", "", nil)
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)

	assert.Equal(t, float64(1), testutil.ToFloat64(m.Requests.WithLabelValues("http", "unmatched", "405")))
	assert.Equal(t, 1, testutil.CollectAndCount(m.Requests))
}

// Unknown methods and paths are client-controlled, so they must all collapse
// into one label value or the series count is unbounded.
func TestMetricsUnmatchedRequestsShareOneSeries(t *testing.T) {
	srv, m := newTestServerWithMetrics(t, &fakeEmbeddingService{})

	do(t, srv, http.MethodGet, "/one", "", nil)
	do(t, srv, http.MethodPost, "/two", "", nil)
	do(t, srv, "FOO", "/three", "", nil)

	assert.Equal(t, 1, testutil.CollectAndCount(m.Duration))
	assert.Equal(t, uint64(3), observationCount(t, m, "http", "unmatched"))
}

// observationCount returns how many observations the duration histogram has
// recorded for the given labels.
func observationCount(t *testing.T, m *metrics.Metrics, labels ...string) uint64 {
	t.Helper()

	var out dto.Metric
	obs, ok := m.Duration.WithLabelValues(labels...).(prometheus.Metric)
	require.True(t, ok, "histogram observer should be a metric")
	require.NoError(t, obs.Write(&out))
	return out.GetHistogram().GetSampleCount()
}
