package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/SaiNageswarS/go-api-boot/async"
)

type AnthropicRequest struct {
	Model       string    `json:"model"`
	MaxTokens   int       `json:"max_tokens"`
	Messages    []Message `json:"messages"`
	System      string    `json:"system,omitempty"`
	Temperature float64   `json:"temperature"`
}

// Message represents a message in the conversation
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AnthropicResponse represents the response from Anthropic API
type AnthropicResponse struct {
	Content []Content `json:"content"`
	ID      string    `json:"id"`
	Model   string    `json:"model"`
	Role    string    `json:"role"`
	Type    string    `json:"type"`
}

// Content represents the content in the response
type Content struct {
	Text string `json:"text"`
	Type string `json:"type"`
}

type AnthropicClient struct {
	apiKey     string
	httpClient *http.Client
	url        string
}

func ProvideAnthropicClient() (*AnthropicClient, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, errors.New("ANTHROPIC_API_KEY environment variable is not set")
	}

	return &AnthropicClient{
		apiKey:     apiKey,
		httpClient: &http.Client{},
		url:        "https://api.anthropic.com/v1/messages",
	}, nil
}

func (c *AnthropicClient) GenerateInference(ctx context.Context, request *AnthropicRequest) <-chan async.Result[string] {
	return async.Go(func() (string, error) {
		jsonData, err := json.Marshal(request)
		if err != nil {
			return "", fmt.Errorf("error marshaling request: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", c.url, bytes.NewBuffer(jsonData))
		if err != nil {
			return "", fmt.Errorf("error creating request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("error making request: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("error reading response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
		}

		var response AnthropicResponse
		if err := json.Unmarshal(body, &response); err != nil {
			return "", fmt.Errorf("error unmarshaling response: %w", err)
		}

		if len(response.Content) == 0 {
			return "", fmt.Errorf("no content in response")
		}

		return response.Content[0].Text, nil
	})
}
