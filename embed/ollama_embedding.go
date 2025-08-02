package embed

import (
	"context"
	"os"
	"time"

	"github.com/SaiNageswarS/go-api-boot/logger"
	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/ollama/ollama/api"
)

type OllamaEmbeddingClient struct {
	cli embeddingsAPI
}

func ProvideOllamaEmbeddingClient() Embedder {
	ollamaHost := os.Getenv("OLLAMA_HOST")
	if ollamaHost == "" {
		// Providers are designed for dependency injection.
		// If the OLLAMA_HOST is not set, we log a fatal error.
		logger.Fatal("OLLAMA_HOST environment variable is not set")
		return nil // This will never be reached, but it's good practice to return nil here.
	}

	ollamaClient, _ := api.ClientFromEnvironment() // api never returns an error

	return &OllamaEmbeddingClient{
		cli: ollamaClient,
	}
}

func (c *OllamaEmbeddingClient) GetEmbedding(ctx context.Context, text string, opts ...EmbedOption) <-chan async.Result[[]float32] {
	return async.Go(func() ([]float32, error) {
		// Default + apply user options
		cfg := settings{model: "nomic-embed-text"}
		for _, opt := range opts {
			opt(&cfg)
		}

		req := api.EmbeddingRequest{
			Model:     cfg.model,
			Prompt:    text,
			KeepAlive: &api.Duration{Duration: 60 * time.Minute}, // keep connection alive for reuse
		}

		resp, err := c.cli.Embeddings(ctx, &req)
		if err != nil {
			return nil, err
		}

		emb64 := resp.Embedding // []float64
		emb32 := make([]float32, len(emb64))
		for i, v := range emb64 {
			emb32[i] = float32(v)
		}
		return emb32, nil
	})
}

type embeddingsAPI interface {
	Embeddings(ctx context.Context, req *api.EmbeddingRequest) (*api.EmbeddingResponse, error)
}
