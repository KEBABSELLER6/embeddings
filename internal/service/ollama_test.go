package service

import (
	"context"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/KEBABSELLER6/embeddings/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testModel = "test-model"

// newTestService starts a fake Ollama server running h and returns a service
// pointed at it. The server is closed when the test ends.
func newTestService(t *testing.T, h http.HandlerFunc, timeout time.Duration) *OllamaService {
	t.Helper()

	return newBatchedTestService(t, h, timeout, 0)
}

// newBatchedTestService is newTestService with a batch size.
func newBatchedTestService(t *testing.T, h http.Handler, timeout time.Duration, batchSize int) *OllamaService {
	t.Helper()

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	cli := &http.Client{Timeout: timeout}
	return NewOllamaService(cli, config.OllamaConfig{Model: testModel, Endpoint: srv.URL}, batchSize)
}

func respondJSON(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

// blockUntilClientGone never answers. It drains the body first because the
// server only notices a disconnected client once the body has been read.
func blockUntilClientGone(_ http.ResponseWriter, r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)
	select {
	case <-r.Context().Done():
	case <-time.After(5 * time.Second):
	}
}

func TestOllamaServiceEmbed(t *testing.T) {
	t.Run("sends the model and input and returns the embeddings", func(t *testing.T) {
		input := []string{"hello", "world"}

		svc := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
			// assert, not require: this runs on the server's goroutine.
			assert.Equal(t, http.MethodPost, r.Method)

			var got OllamaRequest
			assert.NoError(t, json.UnmarshalRead(r.Body, &got))
			assert.Equal(t, OllamaRequest{Model: testModel, Input: input}, got)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			respondJSON(http.StatusOK, `{"model":"test-model","embeddings":[[0.5,0.25],[1,2]]}`)(w, r)
		}, time.Second)

		got, err := svc.Embed(context.Background(), input)

		require.NoError(t, err)
		assert.Equal(t, &common.EmbeddingResponse{
			Model:      testModel,
			Embeddings: [][]float32{{0.5, 0.25}, {1, 2}},
		}, got)
	})

	errTests := []struct {
		name    string
		handler http.HandlerFunc
		wantIs  error
		wantMsg string
	}{
		{
			name:    "model not found",
			handler: respondJSON(http.StatusNotFound, `{"error":"model \"x\" not found, try pulling it first"}`),
			wantIs:  common.ErrUpstream,
			wantMsg: "not found, try pulling it first",
		},
		{
			name:    "server error",
			handler: respondJSON(http.StatusInternalServerError, `{"error":"boom"}`),
			wantIs:  common.ErrUpstream,
			wantMsg: "status 500",
		},
		{
			name:    "malformed json",
			handler: respondJSON(http.StatusOK, `{"embeddings":`),
			wantIs:  common.ErrInvalidOutput,
		},
		{
			name:    "empty body",
			handler: respondJSON(http.StatusOK, ``),
			wantIs:  common.ErrInvalidOutput,
		},
		{
			name:    "fewer embeddings than inputs",
			handler: respondJSON(http.StatusOK, `{"model":"test-model","embeddings":[[1,2]]}`),
			wantIs:  common.ErrInvalidOutput,
		},
		{
			name:    "more embeddings than inputs",
			handler: respondJSON(http.StatusOK, `{"model":"test-model","embeddings":[[1],[2],[3]]}`),
			wantIs:  common.ErrInvalidOutput,
		},
		{
			name:    "no embeddings field",
			handler: respondJSON(http.StatusOK, `{"model":"test-model"}`),
			wantIs:  common.ErrInvalidOutput,
		},
	}

	for _, tt := range errTests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService(t, tt.handler, time.Second)

			got, err := svc.Embed(context.Background(), []string{"a", "b"})

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantIs)
			if tt.wantMsg != "" {
				assert.ErrorContains(t, err, tt.wantMsg)
			}
			assert.Nil(t, got)
		})
	}

	t.Run("truncates a very large upstream error body", func(t *testing.T) {
		svc := newTestService(t, respondJSON(http.StatusBadGateway, strings.Repeat("x", 10000)), time.Second)

		_, err := svc.Embed(context.Background(), []string{"a"})

		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrUpstream)
		assert.Less(t, len(err.Error()), 2000)
	})

	t.Run("client timeout is an upstream timeout", func(t *testing.T) {
		svc := newTestService(t, blockUntilClientGone, 50*time.Millisecond)

		_, err := svc.Embed(context.Background(), []string{"a"})

		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrUpstream)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("context deadline is an upstream timeout", func(t *testing.T) {
		svc := newTestService(t, blockUntilClientGone, 0)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err := svc.Embed(ctx, []string{"a"})

		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrUpstream)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("cancelled context stops the request", func(t *testing.T) {
		svc := newTestService(t, blockUntilClientGone, 0)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := svc.Embed(ctx, []string{"a"})

		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("cancel mid-request is both upstream and canceled", func(t *testing.T) {
		svc := newTestService(t, blockUntilClientGone, 0)

		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(50*time.Millisecond, cancel)

		_, err := svc.Embed(ctx, []string{"a"})

		// Both match, so HandleError must check Canceled before ErrUpstream.
		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrUpstream)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("unreachable server is an upstream error", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		endpoint := srv.URL
		srv.Close()

		svc := NewOllamaService(&http.Client{Timeout: time.Second}, config.OllamaConfig{Model: testModel, Endpoint: endpoint}, 0)

		_, err := svc.Embed(context.Background(), []string{"a"})

		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrUpstream)
	})

	t.Run("invalid endpoint is not reported as an upstream error", func(t *testing.T) {
		svc := NewOllamaService(&http.Client{}, config.OllamaConfig{Model: testModel, Endpoint: "http://bad host/"}, 0)

		_, err := svc.Embed(context.Background(), []string{"a"})

		require.Error(t, err)
		assert.NotErrorIs(t, err, common.ErrUpstream)
		assert.ErrorContains(t, err, "internal error")
	})
}

// fakeOllama answers like a real model: one embedding per input, derived from
// the input so a test can tell where each one ended up. Inputs must be numeric
// strings, and the embedding for "7" is [7]. The reported model is named after
// the chunk's first input, so the chunk a response came from is identifiable.
// It records every chunk it receives and fails the test on an empty one.
type fakeOllama struct {
	t *testing.T

	// delay, if set, is how long a chunk takes, given its first input.
	delay func(first int) time.Duration
	// override, if set, may answer a chunk itself; it returns true if it did.
	override func(w http.ResponseWriter, r *http.Request, input []string) bool

	mu          sync.Mutex
	chunks      [][]string
	inFlight    int
	maxInFlight int
}

func (f *fakeOllama) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// assert, not require: this runs on the server's goroutine.
	var req OllamaRequest
	if !assert.NoError(f.t, json.UnmarshalRead(r.Body, &req)) ||
		!assert.NotEmpty(f.t, req.Input, "empty chunk sent upstream") {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	f.enter(req.Input)
	defer f.exit()

	if f.override != nil && f.override(w, r, req.Input) {
		return
	}

	embeddings := make([][]float32, len(req.Input))
	for i, s := range req.Input {
		n, err := strconv.Atoi(s)
		if !assert.NoError(f.t, err) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		embeddings[i] = []float32{float32(n)}
	}

	if f.delay != nil {
		time.Sleep(f.delay(int(embeddings[0][0])))
	}

	out, err := json.Marshal(OllamaResponse{Model: "model-" + req.Input[0], Embeddings: embeddings})
	assert.NoError(f.t, err)
	_, _ = w.Write(out)
}

func (f *fakeOllama) enter(chunk []string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.chunks = append(f.chunks, chunk)
	f.inFlight++
	f.maxInFlight = max(f.maxInFlight, f.inFlight)
}

func (f *fakeOllama) exit() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.inFlight--
}

func (f *fakeOllama) chunkSizes() []int {
	f.mu.Lock()
	defer f.mu.Unlock()

	sizes := make([]int, len(f.chunks))
	for i, c := range f.chunks {
		sizes[i] = len(c)
	}
	return sizes
}

func (f *fakeOllama) maxConcurrent() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.maxInFlight
}

// numberedInputs returns "0", "1", ... "n-1".
func numberedInputs(n int) []string {
	in := make([]string, n)
	for i := range in {
		in[i] = strconv.Itoa(i)
	}
	return in
}

// numberedEmbeddings is what fakeOllama returns for numberedInputs(n).
func numberedEmbeddings(n int) [][]float32 {
	out := make([][]float32, n)
	for i := range out {
		out[i] = []float32{float32(i)}
	}
	return out
}

func TestOllamaServiceEmbedBatched(t *testing.T) {
	chunkShapes := []struct {
		name       string
		inputs     int
		batchSize  int
		wantChunks []int
	}{
		{"uneven last chunk", 18, 4, []int{4, 4, 4, 4, 2}},
		{"exact multiple of the batch size", 16, 4, []int{4, 4, 4, 4}},
		{"fewer inputs than the batch size", 3, 10, []int{3}},
		{"one input per chunk", 5, 1, []int{1, 1, 1, 1, 1}},
		{"batch size zero sends a single request", 18, 0, []int{18}},
	}

	for _, tt := range chunkShapes {
		t.Run(tt.name+" keeps input order", func(t *testing.T) {
			// Later chunks answer faster, so they finish before earlier ones.
			fake := &fakeOllama{t: t, delay: func(first int) time.Duration {
				return time.Duration(max(0, 30-first)) * time.Millisecond
			}}
			svc := newBatchedTestService(t, fake, time.Second, tt.batchSize)

			got, err := svc.Embed(context.Background(), numberedInputs(tt.inputs))

			require.NoError(t, err)
			assert.Equal(t, numberedEmbeddings(tt.inputs), got.Embeddings)
			assert.Equal(t, "model-0", got.Model, "model should come from the first chunk")
			assert.ElementsMatch(t, tt.wantChunks, fake.chunkSizes())
		})
	}

	t.Run("a failing chunk fails the whole request", func(t *testing.T) {
		fake := &fakeOllama{t: t, override: func(w http.ResponseWriter, r *http.Request, input []string) bool {
			if input[0] != "8" {
				return false
			}
			respondJSON(http.StatusInternalServerError, `{"error":"boom"}`)(w, r)
			return true
		}}
		svc := newBatchedTestService(t, fake, time.Second, 4)

		got, err := svc.Embed(context.Background(), numberedInputs(18))

		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrUpstream)
		assert.ErrorContains(t, err, "status 500")
		assert.Nil(t, got)
	})

	t.Run("a malformed chunk is invalid output, not upstream", func(t *testing.T) {
		fake := &fakeOllama{t: t, override: func(w http.ResponseWriter, r *http.Request, input []string) bool {
			if input[0] != "8" {
				return false
			}
			respondJSON(http.StatusOK, `{"model":"m","embeddings":[]}`)(w, r)
			return true
		}}
		svc := newBatchedTestService(t, fake, time.Second, 4)

		got, err := svc.Embed(context.Background(), numberedInputs(18))

		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrInvalidOutput)
		assert.NotErrorIs(t, err, common.ErrUpstream)
		assert.Nil(t, got)
	})

	t.Run("a failing chunk cancels the chunks still in flight", func(t *testing.T) {
		fake := &fakeOllama{t: t, override: func(w http.ResponseWriter, r *http.Request, input []string) bool {
			if input[0] == "0" {
				respondJSON(http.StatusInternalServerError, `{"error":"boom"}`)(w, r)
			} else {
				blockUntilClientGone(w, r)
			}
			return true
		}}
		// Four chunks, all within the concurrency limit, so all start at once.
		svc := newBatchedTestService(t, fake, 0, 1)

		start := time.Now()
		_, err := svc.Embed(context.Background(), numberedInputs(4))

		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrUpstream)
		// blockUntilClientGone gives up after 5s, so returning sooner means
		// the blocked requests were cancelled rather than waited out.
		assert.Less(t, time.Since(start), 2*time.Second)
	})

	t.Run("cancelling the caller's context stops every chunk", func(t *testing.T) {
		fake := &fakeOllama{t: t, override: func(w http.ResponseWriter, r *http.Request, _ []string) bool {
			blockUntilClientGone(w, r)
			return true
		}}
		svc := newBatchedTestService(t, fake, 0, 2)

		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(50*time.Millisecond, cancel)

		start := time.Now()
		_, err := svc.Embed(ctx, numberedInputs(8))

		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Less(t, time.Since(start), 2*time.Second)
	})

	t.Run("concurrent chunks are capped at the limit", func(t *testing.T) {
		fake := &fakeOllama{t: t, delay: func(int) time.Duration { return 20 * time.Millisecond }}
		svc := newBatchedTestService(t, fake, time.Second, 1)

		got, err := svc.Embed(context.Background(), numberedInputs(goroutineSize*5))

		require.NoError(t, err)
		assert.Equal(t, numberedEmbeddings(goroutineSize*5), got.Embeddings)
		assert.LessOrEqual(t, fake.maxConcurrent(), goroutineSize)
		assert.Greater(t, fake.maxConcurrent(), 1, "chunks should run in parallel")
	})
}
