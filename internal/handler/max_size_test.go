package handler

import (
	"bytes"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaxSizeHandler(t *testing.T) {
	eh := newTestErrorHandler()

	next := func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			eh.HandleError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusOK)
	}

	request := EmbeddingRequest{
		Input: []string{"test123", "test1234", "test"},
	}
	jsonData, err := json.Marshal(request)
	require.NoError(t, err)

	t.Run("returns 200", func(t *testing.T) {
		req := newRequestWithCorrelationID(http.MethodPost, "/api/v1/embed", bytes.NewBuffer(jsonData))
		rr := httptest.NewRecorder()

		handler := NewMaxSizeHandler(512000)
		h := handler.LimitBodySize(next)
		h.ServeHTTP(rr, req)

		assert.Equal(t, 200, rr.Code)
	})

	t.Run("returns 413", func(t *testing.T) {
		req := newRequestWithCorrelationID(http.MethodPost, "/api/v1/embed", bytes.NewBuffer(jsonData))
		rr := httptest.NewRecorder()

		handler := NewMaxSizeHandler(1)
		h := handler.LimitBodySize(next)
		h.ServeHTTP(rr, req)

		assert.Equal(t, 413, rr.Code)
		assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

		var got ErrorResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
		assert.Equal(t, ErrorResponse{Message: "request too large", CorrelationID: testCorrelationID}, got)
	})

	t.Run("body exactly at the limit is accepted", func(t *testing.T) {
		req := newRequestWithCorrelationID(http.MethodPost, "/api/v1/embed", bytes.NewBuffer(jsonData))
		rr := httptest.NewRecorder()

		h := NewMaxSizeHandler(int64(len(jsonData))).LimitBodySize(next)
		h.ServeHTTP(rr, req)

		assert.Equal(t, 200, rr.Code)
	})

	t.Run("body one byte over the limit is rejected", func(t *testing.T) {
		req := newRequestWithCorrelationID(http.MethodPost, "/api/v1/embed", bytes.NewBuffer(jsonData))
		rr := httptest.NewRecorder()

		h := NewMaxSizeHandler(int64(len(jsonData) - 1)).LimitBodySize(next)
		h.ServeHTTP(rr, req)

		assert.Equal(t, 413, rr.Code)
	})
}
