package grpchandler

import (
	"context"

	middleware "github.com/grpc-ecosystem/go-grpc-middleware/v2"
	"google.golang.org/grpc"
)

type StreamCanceller struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func NewStreamCanceller() *StreamCanceller {
	ctx, cancel := context.WithCancel(context.Background())
	return &StreamCanceller{ctx: ctx, cancel: cancel}
}

func (s *StreamCanceller) Cancel() { s.cancel() }

func (s *StreamCanceller) Interceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx, cancel := context.WithCancel(ss.Context())
		defer cancel()
		defer context.AfterFunc(s.ctx, cancel)()

		wrapped := middleware.WrapServerStream(ss)
		wrapped.WrappedContext = ctx
		return handler(srv, wrapped)
	}
}
