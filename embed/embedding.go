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
	taskName  string        //   Jina
}

type EmbedOption func(*settings)

// ---- provider-agnostic helpers ----
func WithModel(name string) EmbedOption {
	return func(s *settings) { s.model = name }
}

// ---- ollama-specific helpers ----
func WithKeepAlive(d time.Duration) EmbedOption {
	return func(s *settings) { s.keepAlive = d }
}

// ---- Jina-specific helpers ----
func WithTask(name string) EmbedOption {
	// e.g. "retrieval.passage" | "retrieval.query"
	return func(s *settings) { s.taskName = name }
}
