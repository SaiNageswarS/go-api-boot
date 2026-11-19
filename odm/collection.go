package odm

import (
	"context"
	"errors"
	"fmt"

	"github.com/SaiNageswarS/go-collection-boot/async"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Collection is a typed collection of T in one tenant. Both backends implement
// it; each adds backend-specific methods on its own Collection type
// (mongodb.Collection adds Aggregate, postgres.Collection adds FindSql).
//
// Every method runs asynchronously and delivers one async.Result.
type Collection[T Model] interface {
	// Save inserts model, or merges its fields into the stored document with
	// the same Id (top-level fields are replaced, fields missing from model
	// are kept). It stamps createdOn on insert and updatedOn on every save.
	Save(ctx context.Context, model T) <-chan async.Result[struct{}]

	// FindByID returns the document with id, or ErrNotFound.
	FindByID(ctx context.Context, id string) <-chan async.Result[*T]
	// FindOne returns a document matching filter, or ErrNotFound. A nil
	// filter matches every document.
	FindOne(ctx context.Context, filter M) <-chan async.Result[*T]
	// Find returns the documents matching filter. A nil filter matches all.
	Find(ctx context.Context, filter M, opts ...FindOption) <-chan async.Result[[]T]
	// Count counts the documents matching filter.
	Count(ctx context.Context, filter M) <-chan async.Result[int64]
	// Exists reports whether a document with id exists.
	Exists(ctx context.Context, id string) <-chan async.Result[bool]
	// Distinct returns the distinct values of field among documents matching
	// filter; array fields contribute their elements. Values are decoded as
	// bson decodes into interface{} (string, int32, int64, float64, ...); use
	// DistinctAs for typed values.
	Distinct(ctx context.Context, field string, filter M) <-chan async.Result[[]any]

	// DeleteByID deletes the document with id, if any.
	DeleteByID(ctx context.Context, id string) <-chan async.Result[struct{}]
	// DeleteOne deletes one document matching filter, if any. filter must not
	// be nil.
	DeleteOne(ctx context.Context, filter M) <-chan async.Result[struct{}]

	// VectorSearch returns the K documents nearest to embedding, using the
	// model's VectorIndexSpec named params.IndexName.
	VectorSearch(ctx context.Context, embedding []float32, params VectorSearchParams) <-chan async.Result[[]SearchHit[T]]
	// TermSearch returns the documents best matching query (any of its words),
	// over the paths of the model's TermSearchIndexSpec named params.IndexName.
	TermSearch(ctx context.Context, query string, params TermSearchParams) <-chan async.Result[[]SearchHit[T]]
}

// SearchHit is a search result with its relevance score (higher is better).
type SearchHit[T Model] struct {
	Score float64 `bson:"score"`
	Doc   T       `bson:"doc"`
}

// VectorSearchParams configures VectorSearch.
type VectorSearchParams struct {
	IndexName     string // a VectorIndexSpec of the model
	K             int    // number of nearest neighbours to return
	NumCandidates int    // approximate candidates to consider; 0 picks a default
	Filter        M      // optional pre-filter
}

// Validate checks the params against an embedding.
func (p VectorSearchParams) Validate(embedding []float32) error {
	if len(embedding) == 0 || p.IndexName == "" || p.K <= 0 {
		return errors.New("odm: VectorSearch needs an embedding, an index name and K > 0")
	}
	if p.NumCandidates < 0 {
		return errors.New("odm: VectorSearch NumCandidates must not be negative")
	}
	return nil
}

// TermSearchParams configures TermSearch.
type TermSearchParams struct {
	IndexName string // a TermSearchIndexSpec of the model
	Limit     int    // number of results to return
	Filter    M      // optional filter
}

// Validate checks the params against a query.
func (p TermSearchParams) Validate(query string) error {
	if query == "" || p.IndexName == "" || p.Limit <= 0 {
		return errors.New("odm: TermSearch needs a query, an index name and Limit > 0")
	}
	return nil
}

// DistinctAs is Distinct with values decoded into V, e.g.
// odm.DistinctAs[string](ctx, movies, "genres", nil).
func DistinctAs[V any, T Model](ctx context.Context, c Collection[T], field string, filter M) <-chan async.Result[[]V] {
	return async.Go(func() ([]V, error) {
		values, err := async.Await(c.Distinct(ctx, field, filter))
		if err != nil {
			return nil, err
		}
		raw, err := bson.Marshal(bson.D{{Key: "v", Value: bson.A(values)}})
		if err != nil {
			return nil, err
		}
		var out struct {
			V []V `bson:"v"`
		}
		if err := bson.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("odm: decoding distinct values of %s: %w", field, err)
		}
		if out.V == nil {
			out.V = []V{}
		}
		return out.V, nil
	})
}
