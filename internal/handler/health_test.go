package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHealthCheckHandler(t *testing.T) {
	t.Run("returns 200 with ok status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rr := httptest.NewRecorder()

		HandleHealthRequest(rr, req)

		assert.Equal(t, 200, rr.Code)
		assert.JSONEq(t, `{"status":"ok"}`, rr.Body.String())
	})
}
