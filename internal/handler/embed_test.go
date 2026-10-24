package handler

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeEmbeddingService struct {
	resp *common.EmbeddingResponse
	err  error

	calls    int
	gotInput []string
}

func (f *fakeEmbeddingService) Embed(_ context.Context, input []string) (*common.EmbeddingResponse, error) {
	f.calls++
	f.gotInput = input
	return f.resp, f.err
}

// testMaxInputs keeps the input-count boundary cases small.
const testMaxInputs = 3

func TestHandleEmbeddingRequest(t *testing.T) {
	okResp := &common.EmbeddingResponse{
		Model:      "test-model",
		Embeddings: [][]float32{{0.5, 0.25}, {1, 2}},
	}

	tests := []struct {
		name        string
		body        string
		svc         *fakeEmbeddingService
		wantStatus  int
		wantCalls   int
		wantInput   []string
		wantJSON    string
		wantErrBody string
	}{
		{
			name:       "valid request",
			body:       `{"input":["hello","world"]}`,
			svc:        &fakeEmbeddingService{resp: okResp},
			wantStatus: http.StatusOK,
			wantCalls:  1,
			wantInput:  []string{"hello", "world"},
			wantJSON:   `{"model":"test-model","embeddings":[[0.5,0.25],[1,2]]}`,
		},
		{
			name:        "malformed json",
			body:        `{nope`,
			svc:         &fakeEmbeddingService{},
			wantStatus:  http.StatusBadRequest,
			wantErrBody: "invalid input",
		},
		{
			name:        "empty body",
			body:        ``,
			svc:         &fakeEmbeddingService{},
			wantStatus:  http.StatusBadRequest,
			wantErrBody: "invalid input",
		},
		{
			name:        "empty input list",
			body:        `{"input":[]}`,
			svc:         &fakeEmbeddingService{},
			wantStatus:  http.StatusBadRequest,
			wantErrBody: "invalid input",
		},
		{
			name:        "missing input field",
			body:        `{}`,
			svc:         &fakeEmbeddingService{},
			wantStatus:  http.StatusBadRequest,
			wantErrBody: "invalid input",
		},
		{
			name:        "too many inputs",
			body:        `{"input":["a","b","c","d"]}`,
			svc:         &fakeEmbeddingService{},
			wantStatus:  http.StatusBadRequest,
			wantErrBody: "invalid input",
		},
		{
			name:       "maximum number of inputs is accepted",
			body:       `{"input":["a","b","c"]}`,
			svc:        &fakeEmbeddingService{resp: okResp},
			wantStatus: http.StatusOK,
			wantCalls:  1,
			wantInput:  []string{"a", "b", "c"},
			wantJSON:   `{"model":"test-model","embeddings":[[0.5,0.25],[1,2]]}`,
		},
		{
			name:        "upstream failure",
			body:        `{"input":["hi"]}`,
			svc:         &fakeEmbeddingService{err: common.ErrUpstream},
			wantStatus:  http.StatusBadGateway,
			wantCalls:   1,
			wantInput:   []string{"hi"},
			wantErrBody: "embedding backend unavailable",
		},
		{
			name:        "wrapped upstream failure",
			body:        `{"input":["hi"]}`,
			svc:         &fakeEmbeddingService{err: fmt.Errorf("%w: status 404", common.ErrUpstream)},
			wantStatus:  http.StatusBadGateway,
			wantCalls:   1,
			wantInput:   []string{"hi"},
			wantErrBody: "embedding backend unavailable",
		},
		{
			name:        "invalid output from backend",
			body:        `{"input":["hi"]}`,
			svc:         &fakeEmbeddingService{err: common.ErrInvalidOutput},
			wantStatus:  http.StatusBadGateway,
			wantCalls:   1,
			wantInput:   []string{"hi"},
			wantErrBody: "invalid output",
		},
		{
			name:        "backend timeout",
			body:        `{"input":["hi"]}`,
			svc:         &fakeEmbeddingService{err: context.DeadlineExceeded},
			wantStatus:  http.StatusGatewayTimeout,
			wantCalls:   1,
			wantInput:   []string{"hi"},
			wantErrBody: "timeout",
		},
		{
			name:        "unknown service error",
			body:        `{"input":["hi"]}`,
			svc:         &fakeEmbeddingService{err: errors.New("boom")},
			wantStatus:  http.StatusInternalServerError,
			wantCalls:   1,
			wantInput:   []string{"hi"},
			wantErrBody: "internal error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewEmbeddingHandler(tt.svc, newTestErrorHandler(), testMaxInputs)
			req := newRequestWithCorrelationID(http.MethodPost, "/api/v1/embed", strings.NewReader(tt.body))
			rr := httptest.NewRecorder()

			h.HandleEmbeddingRequest(rr, req)

			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Equal(t, tt.wantCalls, tt.svc.calls)
			assert.Equal(t, tt.wantInput, tt.svc.gotInput)

			if tt.wantJSON != "" {
				assert.JSONEq(t, tt.wantJSON, rr.Body.String())
				assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
			}
			if tt.wantErrBody != "" {
				assert.Equal(t, tt.wantStatus, rr.Code)
				assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

				var got ErrorResponse
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
				assert.Equal(t, ErrorResponse{Message: tt.wantErrBody, CorrelationID: testCorrelationID}, got)
			}
		})
	}
}

func TestHandleEmbeddingRequestBodyTooLarge(t *testing.T) {
	svc := &fakeEmbeddingService{}
	h := NewEmbeddingHandler(svc, newTestErrorHandler(), testMaxInputs)
	limited := NewMaxSizeHandler(10).LimitBodySize(h.HandleEmbeddingRequest)

	req := newRequestWithCorrelationID(http.MethodPost, "/api/v1/embed", strings.NewReader(`{"input":["hello","world"]}`))
	rr := httptest.NewRecorder()

	limited.ServeHTTP(rr, req)

	assert.Zero(t, svc.calls)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var got ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, ErrorResponse{Message: "request too large", CorrelationID: testCorrelationID}, got)
}
