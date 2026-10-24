package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/stretchr/testify/assert"
)

func TestCorrelationIDMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		incomingHeader string
		wantGenerated  bool
	}{
		{
			name:           "uses existing X-Correlation-ID header",
			incomingHeader: "test-correlation-id",
			wantGenerated:  false,
		},
		{
			name:           "generates id when header is absent",
			incomingHeader: "",
			wantGenerated:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedID string

			handler := CorrelationID(func(w http.ResponseWriter, r *http.Request) {
				capturedID = common.CorrelationIDFromContext(r.Context())
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.incomingHeader != "" {
				req.Header.Set("X-Correlation-ID", tt.incomingHeader)
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assert.NotEmpty(t, capturedID)
			assert.Equal(t, capturedID, rr.Header().Get("X-Correlation-ID"))

			if !tt.wantGenerated {
				assert.Equal(t, tt.incomingHeader, capturedID)
			}
		})
	}
}
