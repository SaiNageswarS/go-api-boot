package odm

import (
	"fmt"
	"time"
)

// A model declares its indexes by implementing any of Indexed,
// TermSearchIndexed and VectorIndexed. Each backend's EnsureCollection creates
// them; it is a deployment step, not something collections do at runtime.

// Indexed declares regular (b-tree) indexes.
type Indexed interface {
	Indexes() []IndexSpec
}

// IndexSpec is a regular index over one or more fields.
type IndexSpec struct {
	Name   string     // optional; derived from the keys when empty
	Keys   []IndexKey // at least one
	Unique bool
	TTL    time.Duration // MongoDB only: expire documents this long after the (date) key
}

// IndexKey is one field of an index.
type IndexKey struct {
	Field string
	Desc  bool
}

// Key is an ascending IndexKey.
func Key(field string) IndexKey { return IndexKey{Field: field} }

// DescKey is a descending IndexKey.
func DescKey(field string) IndexKey { return IndexKey{Field: field, Desc: true} }

// DisplayName is Name, or one derived from the keys (e.g. "year_-1_title_1").
func (s IndexSpec) DisplayName() string {
	if s.Name != "" {
		return s.Name
	}
	name := ""
	for i, k := range s.Keys {
		if i > 0 {
			name += "_"
		}
		dir := 1
		if k.Desc {
			dir = -1
		}
		name += fmt.Sprintf("%s_%d", k.Field, dir)
	}
	return name
}

// TermSearchIndexed declares full-text search indexes.
type TermSearchIndexed interface {
	TermSearchIndexes() []TermSearchIndexSpec
}

// TermSearchIndexSpec is a full-text index over string fields.
type TermSearchIndexSpec struct {
	Name  string
	Paths []string
}

// VectorIndexed declares vector search indexes.
type VectorIndexed interface {
	VectorIndexes() []VectorIndexSpec
}

// Similarity is a vector similarity function.
type Similarity string

const (
	Cosine     Similarity = "cosine"
	DotProduct Similarity = "dotProduct"
	Euclidean  Similarity = "euclidean"
)

// VectorIndexSpec is a vector index over an embedding field (stored as
// bson.Vector or []float32).
type VectorIndexSpec struct {
	Name         string
	Path         string
	Dimensions   int
	Similarity   Similarity // Cosine when empty
	Quantization string     // "", "scalar" or "binary" (MongoDB); "scalar" stores half precision on Postgres
	FilterPaths  []string   // fields VectorSearch filters on
}

// SimilarityOrDefault is Similarity, or Cosine when unset.
func (s VectorIndexSpec) SimilarityOrDefault() Similarity {
	if s.Similarity == "" {
		return Cosine
	}
	return s.Similarity
}

// IndexesOf returns T's regular indexes, if it declares any.
func IndexesOf[T Model]() []IndexSpec {
	var zero T
	if ix, ok := any(zero).(Indexed); ok {
		return ix.Indexes()
	}
	return nil
}

// TermSearchIndexesOf returns T's term search indexes, if it declares any.
func TermSearchIndexesOf[T Model]() []TermSearchIndexSpec {
	var zero T
	if ix, ok := any(zero).(TermSearchIndexed); ok {
		return ix.TermSearchIndexes()
	}
	return nil
}

// VectorIndexesOf returns T's vector indexes, if it declares any.
func VectorIndexesOf[T Model]() []VectorIndexSpec {
	var zero T
	if ix, ok := any(zero).(VectorIndexed); ok {
		return ix.VectorIndexes()
	}
	return nil
}

// VectorIndexOf finds T's vector index by name.
func VectorIndexOf[T Model](name string) (VectorIndexSpec, error) {
	for _, s := range VectorIndexesOf[T]() {
		if s.Name == name {
			return s, nil
		}
	}
	var zero T
	return VectorIndexSpec{}, fmt.Errorf("odm: %s declares no vector index %q", zero.CollectionName(), name)
}

// TermSearchIndexOf finds T's term search index by name.
func TermSearchIndexOf[T Model](name string) (TermSearchIndexSpec, error) {
	for _, s := range TermSearchIndexesOf[T]() {
		if s.Name == name {
			return s, nil
		}
	}
	var zero T
	return TermSearchIndexSpec{}, fmt.Errorf("odm: %s declares no term search index %q", zero.CollectionName(), name)
}
