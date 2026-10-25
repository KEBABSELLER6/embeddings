package handler

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
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
			name:        "client disconnected",
			err:         context.Canceled,
			wantStatus:  statusClientClosedRequest,
			wantMessage: "client disconnected",
		},
		{
			// What the service returns when the client hangs up mid-request.
			name:        "client disconnect takes precedence over upstream failure",
			err:         fmt.Errorf("%w: %w", common.ErrUpstream, context.Canceled),
			wantStatus:  statusClientClosedRequest,
			wantMessage: "client disconnected",
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

func TestHandleErrorLogLevel(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantLevel slog.Level
	}{
		{
			name:      "client disconnect is a warning",
			err:       context.Canceled,
			wantLevel: slog.LevelWarn,
		},
		{
			name:      "client disconnect during upstream call is a warning",
			err:       fmt.Errorf("%w: %w", common.ErrUpstream, context.Canceled),
			wantLevel: slog.LevelWarn,
		},
		{
			name:      "upstream failure is an error",
			err:       common.ErrUpstream,
			wantLevel: slog.LevelError,
		},
		{
			name:      "unknown error is an error",
			err:       errors.New("boom"),
			wantLevel: slog.LevelError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capture := &captureHandler{}
			eh := NewErrorHandler(slog.New(capture))

			eh.HandleError(httptest.NewRecorder(), newRequestWithCorrelationID(http.MethodGet, "/", nil), tt.err)

			require.Len(t, capture.records, 1)
			assert.Equal(t, tt.wantLevel, capture.records[0].Level)
		})
	}
}
