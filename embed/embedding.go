package embed

import (
	"context"
	"time"

	"github.com/SaiNageswarS/go-collection-boot/async"
)

type Embedder interface {
	// GetEmbedding returns a channel that will receive the vector(s)
	// or an error.  Any provider-specific knobs are supplied as options.
	GetEmbedding(
		ctx context.Context,
		text string, // the *thing* to embed
		opts ...EmbedOption, // zero-or-more provider tweaks
	) <-chan async.Result[[]float32]
}

type settings struct {
	model     string        //   common
	keepAlive time.Duration //   ollama
	taskName  string        //   Jina & Gemini
}

// Task name constants (using Jina AI naming convention as standard)
const (
	// Retrieval tasks
	TaskRetrievalQuery   = "retrieval.query"   // For search queries
	TaskRetrievalPassage = "retrieval.passage" // For documents to be retrieved

	// Code-related tasks
	TaskCodeQuery   = "code.query"   // For code search queries
	TaskCodePassage = "code.passage" // For code snippets to be retrieved

	// Text matching and similarity
	TaskTextMatching = "text-matching" // For semantic similarity tasks

	// Additional tasks (mapped to closest Jina equivalent)
	TaskClassification    = "classification"     // For classification tasks
	TaskClustering        = "clustering"         // For clustering tasks
	TaskQuestionAnswering = "question-answering" // For Q&A tasks
	TaskFactVerification  = "fact-verification"  // For fact checking
)

type EmbedOption func(*settings)

// ---- provider-agnostic helpers ----
func WithModel(name string) EmbedOption {
	return func(s *settings) { s.model = name }
}

// ---- ollama-specific helpers ----
func WithKeepAlive(d time.Duration) EmbedOption {
	return func(s *settings) { s.keepAlive = d }
}

// ---- Jina & Gemini task helpers ----
func WithTask(name string) EmbedOption {
	// Supported task names:
	// - TaskRetrievalQuery, TaskRetrievalPassage
	// - TaskCodeQuery, TaskCodePassage
	// - TaskTextMatching, TaskClassification
	// - TaskClustering, TaskQuestionAnswering, TaskFactVerification
	return func(s *settings) { s.taskName = name }
}

// Task-specific helper functions for common use cases
func WithRetrievalQuery() EmbedOption {
	return WithTask(TaskRetrievalQuery)
}

func WithRetrievalPassage() EmbedOption {
	return WithTask(TaskRetrievalPassage)
}

func WithCodeQuery() EmbedOption {
	return WithTask(TaskCodeQuery)
}

func WithCodePassage() EmbedOption {
	return WithTask(TaskCodePassage)
}

func WithTextMatching() EmbedOption {
	return WithTask(TaskTextMatching)
}

func WithClassification() EmbedOption {
	return WithTask(TaskClassification)
}

func WithClustering() EmbedOption {
	return WithTask(TaskClustering)
}

func WithQuestionAnswering() EmbedOption {
	return WithTask(TaskQuestionAnswering)
}

func WithFactVerification() EmbedOption {
	return WithTask(TaskFactVerification)
}
