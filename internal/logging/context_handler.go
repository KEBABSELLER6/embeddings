package logging

import (
	"context"
	"log/slog"
)

type ContextHandler struct {
	inner slog.Handler
}

func NewContextHandler(inner slog.Handler) slog.Handler {
	return ContextHandler{inner: inner}
}

func (h ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	id := CorrelationIDFromContext(ctx)
	if id != "" {
		r.AddAttrs(slog.String("correlation_id", id))
	}
	return h.inner.Handle(ctx, r)
}

func (h ContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return ContextHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h ContextHandler) WithGroup(name string) slog.Handler {
	return ContextHandler{inner: h.inner.WithGroup(name)}
}
