package odm

import "time"

// Indexed declares regular (b-tree) indexes. A model declares its indexes by
// implementing any of Indexed, TermSearchIndexed and VectorIndexed; each
// backend's ensure function (mongodb.EnsureCollection, postgres.EnsureTable)
// creates them. That is a deployment step, not something done at runtime.
//
// Index features a backend doesn't support make its ensure function fail with
// an error wrapping errors.ErrUnsupported.
type Indexed interface {
	Indexes() []IndexSpec
}

// IndexSpec is a regular index over one or more fields (MongoDB) or columns
// (Postgres).
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

// TermSearchIndexed declares full-text search indexes.
type TermSearchIndexed interface {
	TermSearchIndexes() []TermSearchIndexSpec
}

// TermSearchIndexSpec is a full-text index over string fields (MongoDB) or
// text columns (Postgres).
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

// VectorIndexSpec is a vector index over an embedding: a bson.Vector or
// []float32 field on MongoDB, a []float32 field (a vector column) on Postgres.
type VectorIndexSpec struct {
	Name         string
	Path         string // the field (MongoDB) or column (Postgres)
	Dimensions   int
	Similarity   Similarity // Cosine when empty
	Quantization string     // "", "scalar" or "binary" (MongoDB); "scalar" stores half precision on Postgres
	FilterPaths  []string   // MongoDB: fields VectorSearch may pre-filter on
}

// SearchHit is a search result with its relevance score (higher is better).
type SearchHit[T Model] struct {
	Score float64 `bson:"score"`
	Doc   T       `bson:"doc"`
}
