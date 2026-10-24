package handler

import (
	"log/slog"
	"net/http"
	"time"
)

type LoggingHandler struct {
	slog *slog.Logger
}

func NewLoggingHandler(logger *slog.Logger) *LoggingHandler {
	return &LoggingHandler{slog: logger}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (h *LoggingHandler) Logging(next func(w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next(rw, r)

		duration := time.Since(start)
		h.slog.InfoContext(r.Context(), "request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", duration.Milliseconds(),
		)
	}
}
