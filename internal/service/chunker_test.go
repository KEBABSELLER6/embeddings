package service

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// echo answers like a real model: one embedding per input, derived from the
// input so a test can tell where each one ended up. Inputs must be numeric
// strings, and the embedding for "7" is [7]. The reported model is named after
// the first input, so the chunk a response came from is identifiable.
func echo(input []string) (*common.EmbeddingResponse, error) {
	if len(input) == 0 {
		return nil, errors.New("empty chunk")
	}

	embeddings := make([][]float32, len(input))
	for i, s := range input {
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, err
		}
		embeddings[i] = []float32{float32(n)}
	}

	return &common.EmbeddingResponse{Model: "model-" + input[0], Embeddings: embeddings}, nil
}

// fakeEmbedder is an in-memory common.EmbeddingService that records the chunks
// it receives and how many ran at once.
type fakeEmbedder struct {
	// delay, if set, is how long a chunk takes, given its first input.
	delay func(first int) time.Duration
	// override, if set, replaces the default echo behaviour.
	override func(ctx context.Context, input []string) (*common.EmbeddingResponse, error)

	mu          sync.Mutex
	calls       [][]string
	inFlight    int
	maxInFlight int
}

func (f *fakeEmbedder) Embed(ctx context.Context, input []string) (*common.EmbeddingResponse, error) {
	f.mu.Lock()
	f.calls = append(f.calls, input)
	f.inFlight++
	f.maxInFlight = max(f.maxInFlight, f.inFlight)
	f.mu.Unlock()

	defer func() {
		f.mu.Lock()
		f.inFlight--
		f.mu.Unlock()
	}()

	if f.override != nil {
		return f.override(ctx, input)
	}

	resp, err := echo(input)
	if err != nil {
		return nil, err
	}

	if f.delay != nil {
		select {
		case <-time.After(f.delay(int(resp.Embeddings[0][0]))):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return resp, nil
}

func (f *fakeEmbedder) callSizes() []int {
	f.mu.Lock()
	defer f.mu.Unlock()

	sizes := make([]int, len(f.calls))
	for i, c := range f.calls {
		sizes[i] = len(c)
	}
	return sizes
}

func (f *fakeEmbedder) maxConcurrent() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.maxInFlight
}

// testUpstreamTimeout is long enough that only the deadline test ever hits it.
const testUpstreamTimeout = time.Minute

// numberedInputs returns "0", "1", ... "n-1".
func numberedInputs(n int) []string {
	in := make([]string, n)
	for i := range in {
		in[i] = strconv.Itoa(i)
	}
	return in
}

// numberedEmbeddings is what echo returns for numberedInputs(n).
func numberedEmbeddings(n int) [][]float32 {
	out := make([][]float32, n)
	for i := range out {
		out[i] = []float32{float32(i)}
	}
	return out
}

func TestChunkerEmbed(t *testing.T) {
	chunkShapes := []struct {
		name       string
		inputs     int
		batchSize  int
		wantChunks []int
	}{
		{"uneven last chunk", 18, 4, []int{4, 4, 4, 4, 2}},
		{"exact multiple of the batch size", 16, 4, []int{4, 4, 4, 4}},
		{"fewer inputs than the batch size", 3, 10, []int{3}},
		{"as many inputs as the batch size", 4, 4, []int{4}},
		{"one input per chunk", 5, 1, []int{1, 1, 1, 1, 1}},
		{"batch size zero sends a single call", 18, 0, []int{18}},
	}

	for _, tt := range chunkShapes {
		t.Run(tt.name+" keeps input order", func(t *testing.T) {
			// Later chunks answer faster, so they finish before earlier ones.
			fake := &fakeEmbedder{delay: func(first int) time.Duration {
				return time.Duration(max(0, 30-first)) * time.Millisecond
			}}

			got, err := NewChunker(fake, tt.batchSize, testUpstreamTimeout).Embed(context.Background(), numberedInputs(tt.inputs))

			require.NoError(t, err)
			assert.Equal(t, numberedEmbeddings(tt.inputs), got.Embeddings)
			assert.Equal(t, "model-0", got.Model, "model should come from the first chunk")
			assert.ElementsMatch(t, tt.wantChunks, fake.callSizes())
		})
	}

	t.Run("a failing chunk fails the whole request", func(t *testing.T) {
		fake := &fakeEmbedder{override: func(_ context.Context, input []string) (*common.EmbeddingResponse, error) {
			if input[0] == "8" {
				return nil, fmt.Errorf("%w: boom", common.ErrUpstream)
			}
			return echo(input)
		}}

		got, err := NewChunker(fake, 4, testUpstreamTimeout).Embed(context.Background(), numberedInputs(18))

		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrUpstream)
		assert.ErrorContains(t, err, "boom")
		assert.ErrorContains(t, err, "chunk [8:12]")
		assert.Nil(t, got)
	})

	t.Run("a chunk with the wrong number of embeddings is invalid output", func(t *testing.T) {
		fake := &fakeEmbedder{override: func(_ context.Context, input []string) (*common.EmbeddingResponse, error) {
			if input[0] == "8" {
				return &common.EmbeddingResponse{Model: "m", Embeddings: [][]float32{{1}}}, nil
			}
			return echo(input)
		}}

		got, err := NewChunker(fake, 4, testUpstreamTimeout).Embed(context.Background(), numberedInputs(18))

		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrInvalidOutput)
		assert.NotErrorIs(t, err, common.ErrUpstream)
		assert.Nil(t, got)
	})

	t.Run("a failing chunk cancels the chunks still in flight", func(t *testing.T) {
		var cancelled atomic.Int32
		fake := &fakeEmbedder{override: func(ctx context.Context, input []string) (*common.EmbeddingResponse, error) {
			if input[0] == "0" {
				return nil, fmt.Errorf("%w: boom", common.ErrUpstream)
			}

			select {
			case <-ctx.Done():
				cancelled.Add(1)
				return nil, ctx.Err()
			case <-time.After(5 * time.Second):
				return nil, errors.New("chunk was never cancelled")
			}
		}}

		// Four chunks, all within the concurrency limit, so all start at once.
		_, err := NewChunker(fake, 1, testUpstreamTimeout).Embed(context.Background(), numberedInputs(4))

		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrUpstream)
		// Embed waits for every chunk, so this is settled by the time it returns.
		assert.EqualValues(t, 3, cancelled.Load())
	})

	t.Run("cancelling the caller's context stops every chunk", func(t *testing.T) {
		var cancelled atomic.Int32
		fake := &fakeEmbedder{override: func(ctx context.Context, _ []string) (*common.EmbeddingResponse, error) {
			select {
			case <-ctx.Done():
				cancelled.Add(1)
				return nil, ctx.Err()
			case <-time.After(5 * time.Second):
				return nil, errors.New("chunk was never cancelled")
			}
		}}

		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(50*time.Millisecond, cancel)

		// Four chunks, all within the concurrency limit, so all start at once.
		_, err := NewChunker(fake, 2, testUpstreamTimeout).Embed(ctx, numberedInputs(8))

		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		assert.EqualValues(t, 4, cancelled.Load())
	})

	t.Run("the upstream timeout covers all chunks, not each one", func(t *testing.T) {
		// Every chunk fits the timeout on its own, but five rounds of them do not.
		fake := &fakeEmbedder{delay: func(int) time.Duration { return 30 * time.Millisecond }}
		n := maxConcurrentChunks * 5

		start := time.Now()
		got, err := NewChunker(fake, 1, 50*time.Millisecond).Embed(context.Background(), numberedInputs(n))

		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Nil(t, got)
		assert.Less(t, time.Since(start), 140*time.Millisecond, "should stop at the deadline, not run every round")
	})

	t.Run("concurrent chunks are capped at the limit", func(t *testing.T) {
		fake := &fakeEmbedder{delay: func(int) time.Duration { return 20 * time.Millisecond }}
		n := maxConcurrentChunks * 5

		got, err := NewChunker(fake, 1, testUpstreamTimeout).Embed(context.Background(), numberedInputs(n))

		require.NoError(t, err)
		assert.Equal(t, numberedEmbeddings(n), got.Embeddings)
		assert.LessOrEqual(t, fake.maxConcurrent(), maxConcurrentChunks)
		assert.Greater(t, fake.maxConcurrent(), 1, "chunks should run in parallel")
	})

	t.Run("chunks an ollama service over http", func(t *testing.T) {
		var requests atomic.Int32
		svc := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)

			// assert, not require: this runs on the server's goroutine.
			var req OllamaRequest
			assert.NoError(t, json.UnmarshalRead(r.Body, &req))

			resp, err := echo(req.Input)
			if !assert.NoError(t, err) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			out, err := json.Marshal(OllamaResponse{Model: resp.Model, Embeddings: resp.Embeddings})
			assert.NoError(t, err)
			_, _ = w.Write(out)
		}, time.Second)

		got, err := NewChunker(svc, 4, testUpstreamTimeout).Embed(context.Background(), numberedInputs(10))

		require.NoError(t, err)
		assert.Equal(t, numberedEmbeddings(10), got.Embeddings)
		assert.Equal(t, "model-0", got.Model)
		assert.EqualValues(t, 3, requests.Load())
	})
}
