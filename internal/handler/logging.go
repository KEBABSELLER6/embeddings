package handler

import (
	"log/slog"
	"net/http"
	"time"
)

type LoggingHandler struct {
	logger *slog.Logger
}

func NewLoggingHandler(logger *slog.Logger) *LoggingHandler {
	return &LoggingHandler{logger: logger}
}

func (h *LoggingHandler) Logging(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &ResponseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rw, r)

		duration := time.Since(start)
		h.logger.InfoContext(r.Context(), "request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", duration.Milliseconds(),
		)
	}
}
