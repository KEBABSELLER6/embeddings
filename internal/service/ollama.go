package service

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"

	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/KEBABSELLER6/embeddings/internal/config"
	"golang.org/x/sync/errgroup"
)

type OllamaService struct {
	cli       *http.Client
	cfg       config.OllamaConfig
	batchSize int
}

func NewOllamaService(cli *http.Client, cfg config.OllamaConfig, batchSize int) *OllamaService {
	return &OllamaService{cli: cli, cfg: cfg, batchSize: batchSize}
}

type OllamaRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type OllamaResponse struct {
	Model      string      `json:"model"`
	Embeddings [][]float32 `json:"embeddings"`
}

const goroutineSize = 4

func (s *OllamaService) Embed(ctx context.Context, input []string) (*common.EmbeddingResponse, error) {
	result := make([][]float32, len(input))
	var model string
	size := s.batchSize
	if s.batchSize == 0 {
		size = len(input)
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(goroutineSize)

	for start := 0; start < len(input); start += size {
		end := min(start+size, len(input))
		g.Go(func() error {
			d := &OllamaRequest{Model: s.cfg.Model, Input: input[start:end]}
			response, err := getResponse(ctx, s.cli, s.cfg.Endpoint, d)
			if err != nil {
				return err
			}

			copy(result[start:end], response.Embeddings)
			if start == 0 {
				model = response.Model
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

func getResponse(ctx context.Context, cli *http.Client, endpoint string, input *OllamaRequest) (*OllamaResponse, error) {
	b, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed marshal: %w", err)
	}

	body := bytes.NewBuffer(b)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", common.ErrUpstream, err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, e := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
		if e != nil {
			return nil, fmt.Errorf("%w: status %d, reading body: %w", common.ErrUpstream, resp.StatusCode, e)
		}
		return nil, fmt.Errorf("%w: status %d message %s", common.ErrUpstream, resp.StatusCode, string(msg))
	}

	data := &OllamaResponse{}
	if err := json.UnmarshalRead(resp.Body, data); err != nil {
		return nil, fmt.Errorf("%w: %w", common.ErrInvalidOutput, err)
	}

	if len(data.Embeddings) != len(input.Input) {
		return nil, fmt.Errorf("%w: %s", common.ErrInvalidOutput, "output length is malformed")
	}

	return data, nil
}
