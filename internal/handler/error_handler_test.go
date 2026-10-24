package handler

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleError(t *testing.T) {
	eh := newTestErrorHandler()

	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantMessage string
	}{
		{
			name:        "invalid input",
			err:         common.ErrInvalidInput,
			wantStatus:  http.StatusBadRequest,
			wantMessage: "invalid input",
		},
		{
			name:        "wrapped invalid input",
			err:         fmt.Errorf("decode: %w", common.ErrInvalidInput),
			wantStatus:  http.StatusBadRequest,
			wantMessage: "invalid input",
		},
		{
			name:        "request too large",
			err:         &http.MaxBytesError{Limit: 1},
			wantStatus:  http.StatusRequestEntityTooLarge,
			wantMessage: "request too large",
		},
		{
			name:        "request too large wrapped as invalid input",
			err:         fmt.Errorf("%w: %w", common.ErrInvalidInput, &http.MaxBytesError{Limit: 1}),
			wantStatus:  http.StatusRequestEntityTooLarge,
			wantMessage: "request too large",
		},
		{
			name:        "deadline exceeded",
			err:         context.DeadlineExceeded,
			wantStatus:  http.StatusGatewayTimeout,
			wantMessage: "timeout",
		},
		{
			name:        "upstream failure",
			err:         common.ErrUpstream,
			wantStatus:  http.StatusBadGateway,
			wantMessage: "embedding backend unavailable",
		},
		{
			name:        "wrapped upstream failure",
			err:         fmt.Errorf("%w: status %d", common.ErrUpstream, 404),
			wantStatus:  http.StatusBadGateway,
			wantMessage: "embedding backend unavailable",
		},
		{
			name:        "upstream timeout takes precedence over upstream failure",
			err:         fmt.Errorf("%w: %w", common.ErrUpstream, context.DeadlineExceeded),
			wantStatus:  http.StatusGatewayTimeout,
			wantMessage: "timeout",
		},
		{
			name:        "invalid output",
			err:         common.ErrInvalidOutput,
			wantStatus:  http.StatusBadGateway,
			wantMessage: "invalid output",
		},
		{
			name:        "unknown error",
			err:         errors.New("boom"),
			wantStatus:  http.StatusInternalServerError,
			wantMessage: "internal error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newRequestWithCorrelationID(http.MethodGet, "/", nil)
			rr := httptest.NewRecorder()

			eh.HandleError(rr, req, tt.err)

			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

			var got ErrorResponse
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
			assert.Equal(t, ErrorResponse{Message: tt.wantMessage, CorrelationID: testCorrelationID}, got)
		})
	}
}
