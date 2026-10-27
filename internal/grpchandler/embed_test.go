package grpchandler

import (
	"context"
	"errors"
	"fmt"
	"testing"

	embeddingsv1 "github.com/KEBABSELLER6/embeddings/gen/embeddings/v1"
	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestEmbed(t *testing.T) {
	okResp := &common.EmbeddingResponse{
		Model:      "test-model",
		Embeddings: [][]float32{{0.5, 0.25}, {1, 2}},
	}

	tests := []struct {
		name        string
		input       []string
		svc         *fakeEmbeddingService
		wantCode    codes.Code
		wantMessage string
		wantCalls   int
		wantInput   []string
	}{
		{
			name:      "valid request",
			input:     []string{"hello", "world"},
			svc:       &fakeEmbeddingService{resp: okResp},
			wantCode:  codes.OK,
			wantCalls: 1,
			wantInput: []string{"hello", "world"},
		},
		{
			name:      "input count exactly at the limit",
			input:     []string{"a", "b", "c"},
			svc:       &fakeEmbeddingService{resp: okResp},
			wantCode:  codes.OK,
			wantCalls: 1,
			wantInput: []string{"a", "b", "c"},
		},
		{
			name:        "empty input",
			input:       []string{},
			svc:         &fakeEmbeddingService{},
			wantCode:    codes.InvalidArgument,
			wantMessage: "invalid input",
		},
		{
			name:        "missing input",
			input:       nil,
			svc:         &fakeEmbeddingService{},
			wantCode:    codes.InvalidArgument,
			wantMessage: "invalid input",
		},
		{
			name:        "input count one over the limit",
			input:       []string{"a", "b", "c", "d"},
			svc:         &fakeEmbeddingService{},
			wantCode:    codes.InvalidArgument,
			wantMessage: "invalid input",
		},
		{
			name:        "invalid input from the service",
			input:       []string{"hello"},
			svc:         &fakeEmbeddingService{err: common.ErrInvalidInput},
			wantCode:    codes.InvalidArgument,
			wantMessage: "invalid input",
			wantCalls:   1,
			wantInput:   []string{"hello"},
		},
		{
			name:        "upstream failure",
			input:       []string{"hello"},
			svc:         &fakeEmbeddingService{err: common.ErrUpstream},
			wantCode:    codes.Unavailable,
			wantMessage: "embedding backend unavailable",
			wantCalls:   1,
			wantInput:   []string{"hello"},
		},
		{
			name:        "wrapped upstream failure",
			input:       []string{"hello"},
			svc:         &fakeEmbeddingService{err: fmt.Errorf("%w: status 404", common.ErrUpstream)},
			wantCode:    codes.Unavailable,
			wantMessage: "embedding backend unavailable",
			wantCalls:   1,
			wantInput:   []string{"hello"},
		},
		{
			name:        "invalid output",
			input:       []string{"hello"},
			svc:         &fakeEmbeddingService{err: common.ErrInvalidOutput},
			wantCode:    codes.Internal,
			wantMessage: "invalid output",
			wantCalls:   1,
			wantInput:   []string{"hello"},
		},
		{
			name:        "deadline exceeded",
			input:       []string{"hello"},
			svc:         &fakeEmbeddingService{err: context.DeadlineExceeded},
			wantCode:    codes.DeadlineExceeded,
			wantMessage: "timeout",
			wantCalls:   1,
			wantInput:   []string{"hello"},
		},
		{
			name:        "client canceled",
			input:       []string{"hello"},
			svc:         &fakeEmbeddingService{err: context.Canceled},
			wantCode:    codes.Canceled,
			wantMessage: "client disconnected",
			wantCalls:   1,
			wantInput:   []string{"hello"},
		},
		{
			name:        "unknown error does not leak its message",
			input:       []string{"hello"},
			svc:         &fakeEmbeddingService{err: errors.New("secret detail")},
			wantCode:    codes.Internal,
			wantMessage: "internal error",
			wantCalls:   1,
			wantInput:   []string{"hello"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := startTestServer(t, tt.svc)

			resp, err := client.Embed(t.Context(), &embeddingsv1.EmbedRequest{Input: tt.input})

			assert.Equal(t, tt.wantCalls, tt.svc.calls)
			assert.Equal(t, tt.wantInput, tt.svc.gotInput)

			if tt.wantCode != codes.OK {
				require.Error(t, err)
				assert.Nil(t, resp)
				st, ok := status.FromError(err)
				require.True(t, ok, "error should carry a grpc status")
				assert.Equal(t, tt.wantCode, st.Code())
				assert.Equal(t, tt.wantMessage, st.Message())
				return
			}

			require.NoError(t, err)
			assert.Equal(t, "test-model", resp.GetModel())
			require.Len(t, resp.GetEmbeddings(), len(okResp.Embeddings))
			for i, want := range okResp.Embeddings {
				assert.Equal(t, want, resp.GetEmbeddings()[i].GetValues())
			}
		})
	}
}
