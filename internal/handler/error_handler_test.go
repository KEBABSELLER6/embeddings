package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/stretchr/testify/assert"
)

func TestHandleError(t *testing.T) {
	slog.SetDefault(slog.New(slog.DiscardHandler))

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "invalid input",
			err:        common.ErrInvalidInput,
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid input\n",
		},
		{
			name:       "wrapped invalid input",
			err:        fmt.Errorf("decode: %w", common.ErrInvalidInput),
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid input\n",
		},
		{
			name:       "request too large",
			err:        &http.MaxBytesError{Limit: 1},
			wantStatus: http.StatusRequestEntityTooLarge,
			wantBody:   "request too large\n",
		},
		{
			name:       "request too large wrapped as invalid input",
			err:        fmt.Errorf("%w: %w", common.ErrInvalidInput, &http.MaxBytesError{Limit: 1}),
			wantStatus: http.StatusRequestEntityTooLarge,
			wantBody:   "request too large\n",
		},
		{
			name:       "deadline exceeded",
			err:        context.DeadlineExceeded,
			wantStatus: http.StatusGatewayTimeout,
			wantBody:   "timeout\n",
		},
		{
			name:       "upstream failure",
			err:        common.ErrUpstream,
			wantStatus: http.StatusBadGateway,
			wantBody:   "embedding backend unavailable\n",
		},
		{
			name:       "wrapped upstream failure",
			err:        fmt.Errorf("%w: status %d", common.ErrUpstream, 404),
			wantStatus: http.StatusBadGateway,
			wantBody:   "embedding backend unavailable\n",
		},
		{
			name:       "upstream timeout takes precedence over upstream failure",
			err:        fmt.Errorf("%w: %w", common.ErrUpstream, context.DeadlineExceeded),
			wantStatus: http.StatusGatewayTimeout,
			wantBody:   "timeout\n",
		},
		{
			name:       "invalid output",
			err:        common.ErrInvalidOutput,
			wantStatus: http.StatusBadGateway,
			wantBody:   "invalid output\n",
		},
		{
			name:       "unknown error",
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   "internal error\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rr := httptest.NewRecorder()

			HandleError(rr, req, tt.err)

			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Equal(t, tt.wantBody, rr.Body.String())
		})
	}
}
