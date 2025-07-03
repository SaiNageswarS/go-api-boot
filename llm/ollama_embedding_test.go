package llm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
)

type stubClient struct {
	capturedReq *api.EmbeddingRequest
	response    *api.EmbeddingResponse
	err         error
}

func (s *stubClient) Embeddings(_ context.Context, req *api.EmbeddingRequest) (*api.EmbeddingResponse, error) {
	s.capturedReq = req
	return s.response, s.err
}

func TestGetEmbedding(t *testing.T) {
	tests := []struct {
		name      string
		inputText string
		resp      *api.EmbeddingResponse
		err       error
		wantVec   []float32
		wantErr   bool
	}{
		{
			name:      "successful embedding with float64 to float32 conversion",
			inputText: "Go interfaces are powerful",
			resp:      &api.EmbeddingResponse{Embedding: []float64{1.1, 2.2, 3.3}},
			wantVec:   []float32{1.1, 2.2, 3.3},
		},
		{
			name:      "error from client propagates",
			inputText: "fail me",
			err:       errors.New("mock failure"),
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubClient{
				response: tc.resp,
				err:      tc.err,
			}
			client := OllamaEmbeddingClient{cli: stub}

			res, err := async.Await(client.GetEmbedding(context.Background(), tc.inputText))

			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, res)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.wantVec, res)

			// Validate the request fields
			assert.Equal(t, "nomic-embed-text", stub.capturedReq.Model)
			assert.Equal(t, tc.inputText, stub.capturedReq.Prompt)
			assert.NotNil(t, stub.capturedReq.KeepAlive)
			assert.InDelta(t, 60*time.Minute.Seconds(), stub.capturedReq.KeepAlive.Duration.Seconds(), 1)
		})
	}
}

func TestProvideOllamaEmbeddingClient_Success(t *testing.T) {
	withEnv("OLLAMA_HOST", "http://localhost:11434", func(logger *MockLogger) {
		client := ProvideOllamaEmbeddingClient()
		assert.NotNil(t, client)
	})
}

func TestProvideOllamaEmbeddingClient_Failure(t *testing.T) {
	withEnv("OLLAMA_HOST", "", func(logger *MockLogger) {
		ProvideOllamaEmbeddingClient()
		assert.True(t, logger.isFatalCalled)
		assert.Equal(t, "OLLAMA_HOST environment variable is not set", logger.fatalMsg)
	})
}
