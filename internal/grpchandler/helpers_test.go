package grpchandler

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"testing"

	embeddingsv1 "github.com/KEBABSELLER6/embeddings/gen/embeddings/v1"
	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/KEBABSELLER6/embeddings/internal/logging"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// testMaxInputs keeps the input-count boundary cases small.
const testMaxInputs = 3

// testMaxBodySize is large enough for the small requests in these tests.
const testMaxBodySize = 1024

type fakeEmbeddingService struct {
	resp *common.EmbeddingResponse
	err  error
	// panicWith makes Embed panic, to exercise the recovery interceptor.
	panicWith any

	calls         int
	gotInput      []string
	gotCorrelator string
}

func (f *fakeEmbeddingService) Embed(ctx context.Context, input []string) (*common.EmbeddingResponse, error) {
	f.calls++
	f.gotInput = input
	f.gotCorrelator = logging.CorrelationIDFromContext(ctx)
	if f.panicWith != nil {
		panic(f.panicWith)
	}
	return f.resp, f.err
}

// captureHandler records log records; the server logs from its own goroutines.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler         { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler              { return h }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

// byMessage returns the records logged with msg, in order.
func (h *captureHandler) byMessage(msg string) []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []slog.Record
	for _, r := range h.records {
		if r.Message == msg {
			out = append(out, r)
		}
	}
	return out
}

func recordAttrs(r slog.Record) map[string]slog.Value {
	attrs := map[string]slog.Value{}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value
		return true
	})
	return attrs
}

// startTestServer runs the real server, wired like in main, on an in-memory
// listener and returns a client connected to it.
func startTestServer(t *testing.T, svc common.EmbeddingService) (embeddingsv1.EmbeddingServiceClient, *captureHandler) {
	t.Helper()

	capture := &captureHandler{}
	logger := slog.New(logging.NewContextHandler(capture))

	srv := NewServer(Handlers{
		Embed:       NewEmbeddingHandler(svc, NewErrorHandler(logger), testMaxInputs),
		Logging:     NewLoggingHandler(logger),
		Correlation: NewCorrelationHandler(logger),
		MaxBodySize: testMaxBodySize,
	})

	lis := bufconn.Listen(1 << 20)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return embeddingsv1.NewEmbeddingServiceClient(conn), capture
}
