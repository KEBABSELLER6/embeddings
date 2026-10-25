package logging

import "context"

type contextKey string

var (
	correlationKey contextKey = "correlation-key"
)

func CorrelationIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(correlationKey).(string)
	return id
}

func WithCorrelationID(ctx context.Context, id string) context.Context {
	context := context.WithValue(ctx, correlationKey, id)
	return context
}
