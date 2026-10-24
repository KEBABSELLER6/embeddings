package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/stretchr/testify/assert"
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

func TestHandleEmbeddingRequest(t *testing.T) {
	slog.SetDefault(slog.New(slog.DiscardHandler))

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
			wantErrBody: "invalid input\n",
		},
		{
			name:        "empty body",
			body:        ``,
			svc:         &fakeEmbeddingService{},
			wantStatus:  http.StatusBadRequest,
			wantErrBody: "invalid input\n",
		},
		{
			name:        "empty input list",
			body:        `{"input":[]}`,
			svc:         &fakeEmbeddingService{},
			wantStatus:  http.StatusBadRequest,
			wantErrBody: "invalid input\n",
		},
		{
			name:        "missing input field",
			body:        `{}`,
			svc:         &fakeEmbeddingService{},
			wantStatus:  http.StatusBadRequest,
			wantErrBody: "invalid input\n",
		},
		{
			name:        "too many inputs",
			body:        `{"input":[` + strings.TrimSuffix(strings.Repeat(`"a",`, 2049), ",") + `]}`,
			svc:         &fakeEmbeddingService{},
			wantStatus:  http.StatusBadRequest,
			wantErrBody: "invalid input\n",
		},
		{
			name:       "maximum number of inputs is accepted",
			body:       `{"input":[` + strings.TrimSuffix(strings.Repeat(`"a",`, 2048), ",") + `]}`,
			svc:        &fakeEmbeddingService{resp: okResp},
			wantStatus: http.StatusOK,
			wantCalls:  1,
			wantInput:  strings.Split(strings.TrimSuffix(strings.Repeat("a,", 2048), ","), ","),
			wantJSON:   `{"model":"test-model","embeddings":[[0.5,0.25],[1,2]]}`,
		},
		{
			name:        "upstream failure",
			body:        `{"input":["hi"]}`,
			svc:         &fakeEmbeddingService{err: common.ErrUpstream},
			wantStatus:  http.StatusBadGateway,
			wantCalls:   1,
			wantInput:   []string{"hi"},
			wantErrBody: "embedding backend unavailable\n",
		},
		{
			name:        "wrapped upstream failure",
			body:        `{"input":["hi"]}`,
			svc:         &fakeEmbeddingService{err: fmt.Errorf("%w: status 404", common.ErrUpstream)},
			wantStatus:  http.StatusBadGateway,
			wantCalls:   1,
			wantInput:   []string{"hi"},
			wantErrBody: "embedding backend unavailable\n",
		},
		{
			name:        "invalid output from backend",
			body:        `{"input":["hi"]}`,
			svc:         &fakeEmbeddingService{err: common.ErrInvalidOutput},
			wantStatus:  http.StatusBadGateway,
			wantCalls:   1,
			wantInput:   []string{"hi"},
			wantErrBody: "invalid output\n",
		},
		{
			name:        "backend timeout",
			body:        `{"input":["hi"]}`,
			svc:         &fakeEmbeddingService{err: context.DeadlineExceeded},
			wantStatus:  http.StatusGatewayTimeout,
			wantCalls:   1,
			wantInput:   []string{"hi"},
			wantErrBody: "timeout\n",
		},
		{
			name:        "unknown service error",
			body:        `{"input":["hi"]}`,
			svc:         &fakeEmbeddingService{err: errors.New("boom")},
			wantStatus:  http.StatusInternalServerError,
			wantCalls:   1,
			wantInput:   []string{"hi"},
			wantErrBody: "internal error\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewEmbeddingHandler(tt.svc)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/embed", strings.NewReader(tt.body))
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
				assert.Equal(t, tt.wantErrBody, rr.Body.String())
			}
		})
	}
}

func TestHandleEmbeddingRequestBodyTooLarge(t *testing.T) {
	slog.SetDefault(slog.New(slog.DiscardHandler))

	svc := &fakeEmbeddingService{}
	h := NewEmbeddingHandler(svc)
	limited := NewMaxSizeHandler(10).LimitBodySize(h.HandleEmbeddingRequest)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/embed", strings.NewReader(`{"input":["hello","world"]}`))
	rr := httptest.NewRecorder()

	limited.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
	assert.Equal(t, "request too large\n", rr.Body.String())
	assert.Zero(t, svc.calls)
}
