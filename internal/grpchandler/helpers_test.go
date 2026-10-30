package grpchandler

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"testing"

	embeddingsv1 "github.com/KEBABSELLER6/embeddings/gen/embeddings/v1"
	"github.com/KEBABSELLER6/embeddings/internal/auth"
	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/KEBABSELLER6/embeddings/internal/logging"
	"github.com/KEBABSELLER6/embeddings/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
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
// listener and returns an embeddings client connected to it.
func startTestServer(t *testing.T, svc common.EmbeddingService) (embeddingsv1.EmbeddingServiceClient, *captureHandler) {
	t.Helper()

	conn, _, capture := startTestServerConn(t, svc)
	return embeddingsv1.NewEmbeddingServiceClient(conn), capture
}

// startTestServerConn is startTestServer for tests that need the raw
// connection (to build other clients) or the health server.
func startTestServerConn(t *testing.T, svc common.EmbeddingService) (*grpc.ClientConn, *health.Server, *captureHandler) {
	t.Helper()

	ts := startTestServerFull(t, svc)
	return ts.conn, ts.hs, ts.capture
}

type testServer struct {
	srv     *grpc.Server
	conn    *grpc.ClientConn
	hs      *health.Server
	streams *StreamCanceller
	capture *captureHandler
	metrics *metrics.Metrics
}

// startTestServerFull exposes everything the other helpers hide, including
// the request metrics, backed by a registry private to this test.
func startTestServerFull(t *testing.T, svc common.EmbeddingService) *testServer {
	t.Helper()

	return startTestServerWithAPIKey(t, svc, "")
}

// startTestServerWithAPIKey requires apiKey on non-health calls, like main
// does with API_KEY set. An empty apiKey turns auth off.
func startTestServerWithAPIKey(t *testing.T, svc common.EmbeddingService, apiKey string) *testServer {
	t.Helper()

	capture := &captureHandler{}
	logger := slog.New(logging.NewContextHandler(capture))
	m := metrics.New(prometheus.NewRegistry())
	streams := NewStreamCanceller()
	eh := NewErrorHandler(logger)

	srv, hs := NewServer(Handlers{
		Embed:             NewEmbeddingHandler(svc, eh, testMaxInputs),
		Logging:           NewLoggingHandler(logger),
		Correlation:       NewCorrelationHandler(logger),
		MetricsHandler:    NewMetricsHandler(m),
		MaxBodySize:       testMaxBodySize,
		Logger:            logger,
		StreamCanceller:   streams,
		APIKeyAuthHandler: NewAPIKeyAuth(auth.NewAPIKeyService(apiKey), apiKey != "", eh),
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

	return &testServer{srv: srv, conn: conn, hs: hs, streams: streams, capture: capture, metrics: m}
}

// observationCount returns how many observations the duration histogram has
// recorded for the given labels.
func observationCount(t *testing.T, m *metrics.Metrics, labels ...string) uint64 {
	t.Helper()

	var out dto.Metric
	obs, ok := m.Duration.WithLabelValues(labels...).(prometheus.Metric)
	require.True(t, ok, "histogram observer should be a metric")
	require.NoError(t, obs.Write(&out))
	return out.GetHistogram().GetSampleCount()
}
