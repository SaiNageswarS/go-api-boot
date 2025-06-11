package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/SaiNageswarS/go-api-boot/async"
)

type JinaAIEmbeddingRequest struct {
	Model string   `json:"model"` // jina-embeddings-v3
	Task  string   `json:"task"`  // retrieval.passage or retrieval.query
	Input []string `json:"input"`
}

type JinaAIEmbeddingClient struct {
	apiKey     string
	httpClient *http.Client
	url        string
}

func ProvideJinaAIEmbeddingClient() (*JinaAIEmbeddingClient, error) {
	apiKey := os.Getenv("JINA_AI_API_KEY")
	if apiKey == "" {
		return nil, errors.New("JINA_AI_API_KEY environment variable is not set")
	}

	return &JinaAIEmbeddingClient{
		apiKey:     apiKey,
		httpClient: &http.Client{},
		url:        "https://api.jina.ai/v1/embeddings",
	}, nil
}

func (c *JinaAIEmbeddingClient) GetEmbedding(ctx context.Context, req JinaAIEmbeddingRequest) <-chan async.Result[[]float32] {
	return async.Go(func() ([]float32, error) {
		if req.Model == "" {
			req.Model = "jina-embeddings-v3"
		}
		if req.Task == "" {
			req.Task = "retrieval.passage"
		}

		jsonData, err := json.Marshal(req)
		if err != nil {
			return nil, err
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, err
		}

		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to get embedding: %s", resp.Status)
		}

		var result struct {
			Data []struct {
				Embedding []float32 `json:"embedding"`
			} `json:"data"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}

		if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
			return nil, errors.New("no embedding data found")
		}

		return result.Data[0].Embedding, nil
	})
}
