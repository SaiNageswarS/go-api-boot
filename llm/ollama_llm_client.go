package llm

import (
	"context"
	"os"

	"github.com/SaiNageswarS/go-api-boot/logger"
	"github.com/ollama/ollama/api"
)

type OllamaLLMClient struct {
	cli chatAPI
}

// ProvideOllamaClient creates a new Ollama LLM client
func ProvideOllamaClient() LLMClient {
	ollamaHost := os.Getenv("OLLAMA_HOST")
	if ollamaHost == "" {
		logger.Fatal("OLLAMA_HOST environment variable is not set")
		return nil
	}

	ollamaClient, _ := api.ClientFromEnvironment()
	return &OllamaLLMClient{cli: ollamaClient}
}

func (c *OllamaLLMClient) GenerateInference(ctx context.Context, messages []Message, callback func(chunk string) error, opts ...LLMOption) error {
	// Default settings
	settings := LLMSettings{
		model:       "llama3.2",
		temperature: 0.7,
		maxTokens:   4096,
		stream:      false,
	}

	// Apply options
	for _, opt := range opts {
		opt(&settings)
	}

	// Convert messages to Ollama format
	ollamaMessages := make([]api.Message, len(messages))
	for i, msg := range messages {
		ollamaMessages[i] = api.Message{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	req := &api.ChatRequest{
		Model:    settings.model,
		Messages: ollamaMessages,
		Options: map[string]interface{}{
			"temperature": settings.temperature,
			"num_predict": settings.maxTokens,
		},
		Stream: &settings.stream,
	}

	// Add system prompt if provided
	if settings.system != "" {
		systemMsg := api.Message{
			Role:    "system",
			Content: settings.system,
		}
		req.Messages = append([]api.Message{systemMsg}, req.Messages...)
	}

	responseFunc := func(resp api.ChatResponse) error {
		if resp.Message.Content != "" {
			// Call the user-provided callback with each chunk
			return callback(resp.Message.Content)
		}
		return nil
	}

	return c.cli.Chat(ctx, req, responseFunc)
}

// chatAPI interface for Ollama chat operations
type chatAPI interface {
	Chat(ctx context.Context, req *api.ChatRequest, fn api.ChatResponseFunc) error
}
