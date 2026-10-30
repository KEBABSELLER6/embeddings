package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/KEBABSELLER6/embeddings/internal/metrics"
)

type MetricsHandler struct {
	m *metrics.Metrics
}

func NewMetricsHandler(m *metrics.Metrics) *MetricsHandler {
	return &MetricsHandler{m: m}
}

func (h *MetricsHandler) Metrics(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &ResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		h.m.Duration.WithLabelValues("http", route).Observe(time.Since(start).Seconds())
		h.m.Requests.WithLabelValues("http", route, fmt.Sprint(rw.status)).Inc()
	}
}
