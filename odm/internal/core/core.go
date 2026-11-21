// Package core is what the odm backends share but don't export: reading a
// model's declarations and validating them and the queries made with them.
package core

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/SaiNageswarS/go-api-boot/odm"
)

// Unsupported reports a feature a backend doesn't support; it wraps
// errors.ErrUnsupported.
func Unsupported(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{errors.ErrUnsupported}, args...)...)
}

// CheckID rejects empty ids, which would silently share one document.
func CheckID(id string) error {
	if id == "" {
		return errors.New("model Id() must not be empty")
	}
	return nil
}

// Indexes returns T's regular indexes, if it declares any.
func Indexes[T odm.Model]() []odm.IndexSpec {
	var zero T
	if ix, ok := any(zero).(odm.Indexed); ok {
		return ix.Indexes()
	}
	return nil
}

// TermSearchIndexes returns T's term search indexes, if it declares any.
func TermSearchIndexes[T odm.Model]() []odm.TermSearchIndexSpec {
	var zero T
	if ix, ok := any(zero).(odm.TermSearchIndexed); ok {
		return ix.TermSearchIndexes()
	}
	return nil
}

// VectorIndexes returns T's vector indexes, if it declares any.
func VectorIndexes[T odm.Model]() []odm.VectorIndexSpec {
	var zero T
	if ix, ok := any(zero).(odm.VectorIndexed); ok {
		return ix.VectorIndexes()
	}
	return nil
}

// IndexName is the index's Name, or one derived from its keys, e.g.
// "year_-1_title_1".
func IndexName(s odm.IndexSpec) string {
	if s.Name != "" {
		return s.Name
	}
	parts := make([]string, len(s.Keys))
	for i, k := range s.Keys {
		parts[i] = k.Field + "_" + strconv.Itoa(Direction(k.Desc))
	}
	return strings.Join(parts, "_")
}

// Direction is 1 for ascending and -1 for descending.
func Direction(desc bool) int {
	if desc {
		return -1
	}
	return 1
}

// Similarity is the index's similarity, Cosine when unset.
func Similarity(s odm.VectorIndexSpec) odm.Similarity {
	if s.Similarity == "" {
		return odm.Cosine
	}
	return s.Similarity
}

// ValidateIndex checks a regular index spec.
func ValidateIndex(s odm.IndexSpec) error {
	if len(s.Keys) == 0 {
		return fmt.Errorf("index %q has no keys", s.Name)
	}
	for _, k := range s.Keys {
		if k.Field == "" {
			return fmt.Errorf("index %s has an empty key", IndexName(s))
		}
	}
	return nil
}

// ValidateTermSearchIndex checks a term search index spec.
func ValidateTermSearchIndex(s odm.TermSearchIndexSpec) error {
	if s.Name == "" || len(s.Paths) == 0 || slices.Contains(s.Paths, "") {
		return fmt.Errorf("term search index %q needs a name and non-empty paths", s.Name)
	}
	return nil
}

// ValidateVectorIndex checks a vector index spec.
func ValidateVectorIndex(s odm.VectorIndexSpec) error {
	if s.Name == "" || s.Path == "" || s.Dimensions <= 0 {
		return fmt.Errorf("vector index %q needs a name, a path and dimensions", s.Name)
	}
	switch Similarity(s) {
	case odm.Cosine, odm.DotProduct, odm.Euclidean:
		return nil
	}
	return fmt.Errorf("vector index %s: unknown similarity %q", s.Name, s.Similarity)
}

// CheckVectorQuery checks a vector search's arguments against its index.
func CheckVectorQuery(s odm.VectorIndexSpec, embedding []float32, k int) error {
	if len(embedding) == 0 || k <= 0 {
		return errors.New("vector search needs an embedding and k > 0")
	}
	if len(embedding) != s.Dimensions {
		return fmt.Errorf("index %s has %d dimensions, query has %d", s.Name, s.Dimensions, len(embedding))
	}
	return nil
}

// CheckTermQuery checks a term search's arguments.
func CheckTermQuery(query string, limit int) error {
	if query == "" || limit <= 0 {
		return errors.New("term search needs a query and limit > 0")
	}
	return nil
}
