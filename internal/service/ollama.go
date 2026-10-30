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
)

type OllamaService struct {
	cli *http.Client
	cfg config.OllamaConfig
}

func NewOllamaService(cli *http.Client, cfg config.OllamaConfig) *OllamaService {
	return &OllamaService{cli: cli, cfg: cfg}
}

type OllamaRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type OllamaResponse struct {
	Model      string      `json:"model"`
	Embeddings [][]float32 `json:"embeddings"`
}

func (s *OllamaService) Embed(ctx context.Context, input []string) (*common.EmbeddingResponse, error) {
	d := &OllamaRequest{Model: s.cfg.Model, Input: input}
	b, err := json.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("failed marshal: %w", err)
	}

	body := bytes.NewBuffer(b)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.Endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.cli.Do(req)
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

	if len(data.Embeddings) != len(input) {
		return nil, fmt.Errorf("%w: %s", common.ErrInvalidOutput, "output length is malformed")
	}

	res := &common.EmbeddingResponse{
		Model:      data.Model,
		Embeddings: data.Embeddings,
	}
	return res, nil
}
