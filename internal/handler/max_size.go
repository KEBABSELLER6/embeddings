package handler

import "net/http"

type MaxSizeHandler struct {
	limit int64
}

func NewMaxSizeHandler(limit int64) *MaxSizeHandler {
	return &MaxSizeHandler{limit: limit}
}

func (h *MaxSizeHandler) LimitBodySize(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, h.limit)
		next.ServeHTTP(w, r)
	}
}
