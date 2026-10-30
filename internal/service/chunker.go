package service

import (
	"context"
	"fmt"

	"github.com/KEBABSELLER6/embeddings/internal/common"
	"golang.org/x/sync/errgroup"
)

type Chunker struct {
	inner     common.EmbeddingService
	batchSize int
}

func NewChunker(inner common.EmbeddingService, batchSize int) *Chunker {
	return &Chunker{inner: inner, batchSize: batchSize}
}

const goroutineSize = 4

func (c *Chunker) Embed(ctx context.Context, input []string) (*common.EmbeddingResponse, error) {
	if c.batchSize == 0 || len(input) <= c.batchSize {
		resp, err := c.inner.Embed(ctx, input)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}

	result := make([][]float32, len(input))
	var model string
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(goroutineSize)

	for start := 0; start < len(input); start += c.batchSize {
		end := min(start+c.batchSize, len(input))
		g.Go(func() error {
			resp, err := c.inner.Embed(ctx, input[start:end])
			if err != nil {
				return err
			}

			if len(resp.Embeddings) != end-start {
				return fmt.Errorf("%w: %s", common.ErrInvalidOutput, "output length is malformed")
			}

			copy(result[start:end], resp.Embeddings)
			if start == 0 {
				model = resp.Model
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	res := &common.EmbeddingResponse{
		Model:      model,
		Embeddings: result,
	}

	return res, nil
}
