package service

import (
	"context"
	"fmt"
	"net/http"

	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/KEBABSELLER6/embeddings/internal/config"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type OpenAIService struct {
	cli openai.Client
	cfg config.OpenAIConfig
}

func NewOpenAIService(cli *http.Client, cfg config.OpenAIConfig) *OpenAIService {
	client := openai.NewClient(
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(cfg.BaseURL),
		option.WithHTTPClient(cli),
		option.WithMaxRetries(0),
	)
	return &OpenAIService{cli: client, cfg: cfg}
}

func (s *OpenAIService) Embed(ctx context.Context, input []string) (*common.EmbeddingResponse, error) {
	resp, err := s.cli.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: input,
		},
		Model:          s.cfg.Model,
		EncodingFormat: openai.EmbeddingNewParamsEncodingFormatFloat,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", common.ErrUpstream, err)
	}

	if len(resp.Data) != len(input) {
		return nil, fmt.Errorf("%w: %s", common.ErrInvalidOutput, "output length is malformed")
	}

	result := make([][]float32, len(input))
	seen := make([]bool, len(input))
	for _, e := range resp.Data {
		if e.Index < 0 || e.Index >= int64(len(input)) || seen[e.Index] {
			return nil, fmt.Errorf("%w: bad embedding index %d", common.ErrInvalidOutput, e.Index)
		}
		seen[e.Index] = true

		conv := make([]float32, len(e.Embedding))
		for i, f := range e.Embedding {
			conv[i] = float32(f)
		}

		result[e.Index] = conv
	}

	res := &common.EmbeddingResponse{
		Model:      resp.Model,
		Embeddings: result,
	}

	return res, nil
}
