package llm

import (
	"context"
	"testing"

	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
)

func TestProvideOllamaClient_MissingAPIKey(t *testing.T) {
	withEnv("OLLAMA_HOST", "", func(logger *MockLogger) {
		ProvideOllamaClient()

		assert.True(t, logger.isFatalCalled)
		assert.Equal(t, "OLLAMA_HOST environment variable is not set", logger.fatalMsg)
	})
}

func TestProvideOllamaClient_Success(t *testing.T) {
	withEnv("OLLAMA_HOST", "http://localhost:11434", func(logger *MockLogger) {
		client := ProvideOllamaClient()
		assert.NotNil(t, client)
	})
}

func TestOllamaLLMClient_Success(t *testing.T) {
	client := &OllamaLLMClient{
		cli: &mockChatAPI{
			mockResponse: "Hello, this is a mock response",
		},
	}

	inputMessages := []Message{
		{Role: "user", Content: "Hello"},
	}

	err := client.GenerateInference(t.Context(), inputMessages, func(chunk string) error {
		assert.Equal(t, "Hello, this is a mock response", chunk)
		return nil
	})
	assert.NoError(t, err)
}

func TestOllamaLLMClient_Settings(t *testing.T) {
	client := &OllamaLLMClient{
		cli: &mockChatAPI{
			mockResponse: "Settings applied",
		},
	}

	inputMessages := []Message{
		{Role: "user", Content: "Test settings"},
	}

	err := client.GenerateInference(t.Context(), inputMessages, func(chunk string) error {
		assert.Equal(t, "Settings applied", chunk)
		return nil
	}, WithLLMModel("llama3.2"), WithTemperature(0.7), WithSystemPrompt("You are test agent"))

	assert.NoError(t, err)
	assert.Equal(t, "llama3.2", client.cli.(*mockChatAPI).reqReceived.Model)
	assert.Equal(t, 0.7, client.cli.(*mockChatAPI).reqReceived.Options["temperature"])
	assert.Equal(t, "You are test agent", client.cli.(*mockChatAPI).reqReceived.Messages[0].Content)
	assert.Equal(t, "system", client.cli.(*mockChatAPI).reqReceived.Messages[0].Role)
}

type mockChatAPI struct {
	mockResponse string
	reqReceived  *api.ChatRequest
}

func (m *mockChatAPI) Chat(ctx context.Context, req *api.ChatRequest, callback api.ChatResponseFunc) error {
	// Simulate a successful chat response
	m.reqReceived = req
	response := api.ChatResponse{
		Message: api.Message{
			Content: m.mockResponse,
		},
	}
	return callback(response)
}
