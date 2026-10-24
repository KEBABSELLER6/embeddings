package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"uuid"

	"github.com/KEBABSELLER6/embeddings/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			name:           "keeps id with every allowed character class",
			incomingHeader: "abc-XYZ_123.x",
			wantGenerated:  false,
		},
		{
			name:           "keeps id exactly at the length limit",
			incomingHeader: strings.Repeat("a", maxCorrelationIDLen),
			wantGenerated:  false,
		},
		{
			name:           "generates id when header is absent",
			incomingHeader: "",
			wantGenerated:  true,
		},
		{
			name:           "generates id when header is one over the length limit",
			incomingHeader: strings.Repeat("a", maxCorrelationIDLen+1),
			wantGenerated:  true,
		},
		{
			name:           "generates id when header is invalid utf-8",
			incomingHeader: "bad\xffid",
			wantGenerated:  true,
		},
		{
			name:           "generates id when header is valid utf-8 but not ascii",
			incomingHeader: "héllo",
			wantGenerated:  true,
		},
		{
			name:           "generates id when header contains a space",
			incomingHeader: "has space",
			wantGenerated:  true,
		},
		{
			name:           "generates id when header contains a line break",
			incomingHeader: "line\nbreak",
			wantGenerated:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedID string

			handler := CorrelationID(func(w http.ResponseWriter, r *http.Request) {
				capturedID = logging.CorrelationIDFromContext(r.Context())
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

			if tt.wantGenerated {
				assert.NotEqual(t, tt.incomingHeader, capturedID)
				_, err := uuid.Parse(capturedID)
				require.NoError(t, err, "generated id should be a uuid")
			} else {
				assert.Equal(t, tt.incomingHeader, capturedID)
			}
		})
	}
}
