package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/SaiNageswarS/go-api-boot/async"
	"github.com/stretchr/testify/assert"
)

func TestProvideJinaAIEmbeddingClient_Success(t *testing.T) {
	originalApiKey := os.Getenv("JINA_AI_API_KEY")
	os.Setenv("JINA_AI_API_KEY", "dummy-key")
	defer os.Setenv("JINA_AI_API_KEY", originalApiKey)

	client, err := ProvideJinaAIEmbeddingClient()
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, "dummy-key", client.apiKey)
}

func TestProvideJinaAIEmbeddingClient_MissingAPIKey(t *testing.T) {
	originalApiKey := os.Getenv("JINA_AI_API_KEY")
	os.Unsetenv("JINA_AI_API_KEY")
	defer os.Setenv("JINA_AI_API_KEY", originalApiKey)

	client, err := ProvideJinaAIEmbeddingClient()
	assert.Nil(t, client)
	assert.EqualError(t, err, "JINA_AI_API_KEY environment variable is not set")
}

func TestGetEmbedding_Success(t *testing.T) {
	mockResponse := `{
		"data": [
			{"embedding": [0.1, 0.2, 0.3]}
		]
	}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, mockResponse)
	}))
	defer server.Close()

	client := &JinaAIEmbeddingClient{
		apiKey:     "test-key",
		httpClient: server.Client(),
		url:        server.URL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req := JinaAIEmbeddingRequest{
		Input: []string{"Hello, world"},
	}

	result, err := async.Await(client.GetEmbedding(ctx, req))

	assert.NoError(t, err)
	assert.Equal(t, []float64{0.1, 0.2, 0.3}, result)
}

func TestGetEmbedding_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Bad Request", http.StatusBadRequest)
	}))
	defer server.Close()

	client := &JinaAIEmbeddingClient{
		apiKey:     "test-key",
		httpClient: server.Client(),
		url:        server.URL,
	}

	ctx := context.Background()
	req := JinaAIEmbeddingRequest{Input: []string{"test"}}
	result := <-client.GetEmbedding(ctx, req)

	assert.Error(t, result.Err)
	assert.Contains(t, result.Err.Error(), "failed to get embedding")
}

func TestGetEmbedding_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("invalid-json"))
	}))
	defer server.Close()

	client := &JinaAIEmbeddingClient{
		apiKey:     "test-key",
		httpClient: server.Client(),
		url:        server.URL,
	}

	ctx := context.Background()
	req := JinaAIEmbeddingRequest{Input: []string{"test"}}
	result := <-client.GetEmbedding(ctx, req)

	assert.Error(t, result.Err)
	assert.Contains(t, result.Err.Error(), "invalid character")
}

func TestGetEmbedding_EmptyData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data": []}`)
	}))
	defer server.Close()

	client := &JinaAIEmbeddingClient{
		apiKey:     "test-key",
		httpClient: server.Client(),
		url:        server.URL,
	}

	ctx := context.Background()
	req := JinaAIEmbeddingRequest{Input: []string{"test"}}
	result := <-client.GetEmbedding(ctx, req)

	assert.Error(t, result.Err)
	assert.EqualError(t, result.Err, "no embedding data found")
}
