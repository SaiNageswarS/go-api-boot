package llm

import (
	"context"
)

type LLMClient interface {
	GenerateInference(
		ctx context.Context,
		messages []Message,
		callback func(chunk string) error,
		opts ...LLMOption,
	) error
}

type LLMSettings struct {
	model       string  // model name
	temperature float64 // randomness (0.0 to 1.0)
	maxTokens   int     // maximum tokens to generate
	system      string  // system prompt
	stream      bool    // whether to stream response
}

type LLMOption func(*LLMSettings)

// Common options for all LLM providers
func WithLLMModel(name string) LLMOption {
	return func(s *LLMSettings) { s.model = name }
}

func WithTemperature(temp float64) LLMOption {
	return func(s *LLMSettings) { s.temperature = temp }
}

func WithMaxTokens(tokens int) LLMOption {
	return func(s *LLMSettings) { s.maxTokens = tokens }
}

func WithSystemPrompt(prompt string) LLMOption {
	return func(s *LLMSettings) { s.system = prompt }
}

func WithStreaming(stream bool) LLMOption {
	return func(s *LLMSettings) { s.stream = stream }
}

type Message struct {
	Role    string `json:"role"`    // "user", "assistant", "system"
	Content string `json:"content"` // the message content
}
