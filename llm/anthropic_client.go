package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/SaiNageswarS/go-api-boot/logger"
)

type AnthropicClient struct {
	apiKey     string
	httpClient *http.Client
	url        string
}

func ProvideAnthropicClient() LLMClient {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		// Providers are designed for dependency injection.
		// If the API key is not set, we log a fatal error.
		logger.Fatal("ANTHROPIC_API_KEY environment variable is not set")
		return nil // This will never be reached, but it's good practice to return nil here.
	}

	return &AnthropicClient{
		apiKey:     apiKey,
		httpClient: &http.Client{},
		url:        "https://api.anthropic.com/v1/messages",
	}
}

func (c *AnthropicClient) GenerateInference(ctx context.Context, messages []Message, callback func(chunk string) error, opts ...LLMOption) error {
	settings := LLMSettings{
		model:       "claude-3-sonnet-20240229",
		temperature: 0.7,
		maxTokens:   4096,
	}

	// Apply options
	for _, opt := range opts {
		opt(&settings)
	}

	request := anthropicRequest{
		Model:       settings.model,
		MaxTokens:   settings.maxTokens,
		Temperature: settings.temperature,
		System:      settings.system,
		Messages:    messages,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var response anthropicResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("error unmarshaling response: %w", err)
	}

	if len(response.Content) == 0 {
		return fmt.Errorf("no content in response")
	}

	return callback(response.Content[0].Text)
}

type anthropicRequest struct {
	Model       string    `json:"model"`
	MaxTokens   int       `json:"max_tokens"`
	Messages    []Message `json:"messages"`
	System      string    `json:"system,omitempty"`
	Temperature float64   `json:"temperature"`
}

// anthropicResponse represents the response from Anthropic API
type anthropicResponse struct {
	Content []content `json:"content"`
	ID      string    `json:"id"`
	Model   string    `json:"model"`
	Role    string    `json:"role"`
	Type    string    `json:"type"`
}

// content represents the content in the response
type content struct {
	Text string `json:"text"`
	Type string `json:"type"`
}
