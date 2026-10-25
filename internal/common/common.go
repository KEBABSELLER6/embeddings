package common

import (
	"errors"
)

var (
	ErrInvalidInput  = errors.New("invalid input")
	ErrUpstream      = errors.New("embedding backend failed")
	ErrInvalidOutput = errors.New("invalid output")
)
