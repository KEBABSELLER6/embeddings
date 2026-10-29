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
	ctx = context.WithValue(ctx, correlationKey, id)
	return ctx
}

const MaxCorrelationIDLen = 128

func ValidCorrelationID(id string) bool {
	if id == "" || len(id) > MaxCorrelationIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		case c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	return true
}
