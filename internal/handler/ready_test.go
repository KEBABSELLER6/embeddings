package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadinessHandler(t *testing.T) {
	t.Run("returns 200 with ok status before draining", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rr := httptest.NewRecorder()

		NewReadiness().HandleReadyRequest(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
		assert.JSONEq(t, `{"status":"ok"}`, rr.Body.String())
	})

	t.Run("returns 503 with down status while draining", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rr := httptest.NewRecorder()
		ready := NewReadiness()

		ready.SetDraining()
		ready.HandleReadyRequest(rr, req)

		assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
		// Set before WriteHeader, or the recorder (like a real server) drops it.
		assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
		assert.JSONEq(t, `{"status":"down"}`, rr.Body.String())
	})
}
