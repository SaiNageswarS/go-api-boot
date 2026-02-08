package embed

import (
	"context"
	"errors"
	"os"

	"github.com/SaiNageswarS/go-api-boot/logger"
	"github.com/SaiNageswarS/go-collection-boot/async"
	"google.golang.org/genai"
)

type GeminiEmbeddingClient struct {
	client genaiClientInterface
}

func ProvideGeminiEmbeddingClient() Embedder {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		// Providers are designed for dependency injection.
		// If the API key is not set, we log a fatal error.
		logger.Fatal("GEMINI_API_KEY environment variable is not set")
		return nil // This will never be reached, but it's good practice to return nil here.
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		logger.Fatal("Failed to create Gemini client: " + err.Error())
		return nil
	}

	return &GeminiEmbeddingClient{
		client: &genaiClientWrapper{client: client},
	}
}

func (c *GeminiEmbeddingClient) GetEmbedding(ctx context.Context, text string, opts ...EmbedOption) <-chan async.Result[[]float32] {
	return async.Go(func() ([]float32, error) {
		cfg := settings{model: "text-embedding-004", taskName: TaskTextMatching}
		for _, opt := range opts {
			opt(&cfg)
		}

		// Create content from text
		content := genai.NewContentFromText(text, genai.RoleUser)
		contents := []*genai.Content{content}

		// Create embed content config with optional task type
		config := &genai.EmbedContentConfig{}
		// Map Jina task names to Gemini task type strings
		config.TaskType = mapJinaTaskToGemini(cfg.taskName)

		// Call the embedding API
		result, err := c.client.EmbedContent(ctx, cfg.model, contents, config)
		if err != nil {
			return nil, err
		}

		if len(result.Embeddings) == 0 || len(result.Embeddings[0].Values) == 0 {
			return nil, errors.New("no embedding data found")
		}

		return result.Embeddings[0].Values, nil
	})
}

// genaiClientInterface allows for testing by providing a mock interface
type genaiClientInterface interface {
	EmbedContent(ctx context.Context, model string, contents []*genai.Content, config *genai.EmbedContentConfig) (*genai.EmbedContentResponse, error)
}

// genaiClientWrapper wraps the actual genai.Client to implement the interface
type genaiClientWrapper struct {
	client *genai.Client
}

func (w *genaiClientWrapper) EmbedContent(ctx context.Context, model string, contents []*genai.Content, config *genai.EmbedContentConfig) (*genai.EmbedContentResponse, error) {
	return w.client.Models.EmbedContent(ctx, model, contents, config)
}

// mapJinaTaskToGemini maps Jina AI task names to Gemini AI task type strings
func mapJinaTaskToGemini(jinaTask string) string {
	switch jinaTask {
	// Retrieval tasks
	case TaskRetrievalQuery:
		return "RETRIEVAL_QUERY"
	case TaskRetrievalPassage:
		return "RETRIEVAL_DOCUMENT"

	// Code tasks
	case TaskCodeQuery:
		return "CODE_RETRIEVAL_QUERY"
	case TaskCodePassage:
		return "RETRIEVAL_DOCUMENT" // Map to document retrieval as closest equivalent

	// Text matching and similarity
	case TaskTextMatching:
		return "SEMANTIC_SIMILARITY"

	// Additional tasks
	case TaskClassification:
		return "CLASSIFICATION"
	case TaskClustering:
		return "CLUSTERING"
	case TaskQuestionAnswering:
		return "QUESTION_ANSWERING"
	case TaskFactVerification:
		return "FACT_VERIFICATION"

	// Default fallback
	default:
		return "SEMANTIC_SIMILARITY"
	}
}
