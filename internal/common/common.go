package common

import (
	"context"
	"errors"
)

type contextKey string

var (
	ErrInvalidInput  = errors.New("invalid input")
	ErrUpstream      = errors.New("embedding backend failed")
	ErrInvalidOutput = errors.New("invalid output")
)

var (
	CorrelationKey contextKey = "correlation-key"
)

func CorrelationIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(CorrelationKey).(string)
	return id
}
