package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/stretchr/testify/assert"
)

func TestProvideAnthropicClient_MissingAPIKey(t *testing.T) {
	withEnv("ANTHROPIC_API_KEY", "", func(logger *MockLogger) {
		ProvideAnthropicClient()

		assert.True(t, logger.isFatalCalled)
		assert.Equal(t, "ANTHROPIC_API_KEY environment variable is not set", logger.fatalMsg)
	})
}

func TestProvideAnthropicClient_Success(t *testing.T) {
	withEnv("ANTHROPIC_API_KEY", "test-key", func(logger *MockLogger) {
		client := ProvideAnthropicClient()

		assert.NotNil(t, client)
		assert.Equal(t, "test-key", client.apiKey)
		assert.NotEmpty(t, client.url)
	})
}

func TestGenerateInference_Success(t *testing.T) {
	mockResponse := `{
		"content": [{"text": "Test response", "type": "text"}],
		"id": "id123", "model": "claude-2", "role": "assistant", "type": "message"
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("Missing or incorrect API key header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	client := &AnthropicClient{
		apiKey:     "test-key",
		httpClient: server.Client(),
		url:        server.URL,
	}

	req := &AnthropicRequest{
		Model:       "claude-2",
		MaxTokens:   50,
		Temperature: 0.5,
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	respText, err := async.Await(client.GenerateInference(context.Background(), req))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if respText != "Test response" {
		t.Errorf("Expected 'Test response', got '%s'", respText)
	}
}

func TestGenerateInference_BadStatusCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	client := &AnthropicClient{
		apiKey:     "test-key",
		httpClient: server.Client(),
		url:        server.URL,
	}

	req := &AnthropicRequest{
		Model:     "claude-2",
		MaxTokens: 50,
		Messages:  []Message{{Role: "user", Content: "Test"}},
	}

	_, err := async.Await(client.GenerateInference(context.Background(), req))
	if err == nil {
		t.Fatal("Expected error due to bad status code, got nil")
	}
}

func TestGenerateInference_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()

	client := &AnthropicClient{
		apiKey:     "test-key",
		httpClient: server.Client(),
		url:        server.URL,
	}

	req := &AnthropicRequest{
		Model:     "claude-2",
		MaxTokens: 50,
		Messages:  []Message{{Role: "user", Content: "Test"}},
	}

	_, err := async.Await(client.GenerateInference(context.Background(), req))
	if err == nil {
		t.Fatal("Expected JSON unmarshal error, got nil")
	}
}

func TestGenerateInference_EmptyContent(t *testing.T) {
	mockResp := `{
		"content": [],
		"id": "id123", "model": "claude-2", "role": "assistant", "type": "message"
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockResp))
	}))
	defer server.Close()

	client := &AnthropicClient{
		apiKey:     "test-key",
		httpClient: server.Client(),
		url:        server.URL,
	}

	req := &AnthropicRequest{
		Model:     "claude-2",
		MaxTokens: 50,
		Messages:  []Message{{Role: "user", Content: "Test"}},
	}

	_, err := async.Await(client.GenerateInference(context.Background(), req))
	if err == nil || err.Error() != "no content in response" {
		t.Errorf("Expected 'no content in response' error, got: %v", err)
	}
}
