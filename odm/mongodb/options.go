package mongodb

import (
	"context"
	"errors"
	"fmt"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-collection-boot/async"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// SortField orders results by one field.
type SortField struct {
	Field string
	Desc  bool
}

// Asc sorts by field, smallest first.
func Asc(field string) SortField { return SortField{Field: field} }

// Desc sorts by field, largest first.
func Desc(field string) SortField { return SortField{Field: field, Desc: true} }

// FindOption configures Find.
type FindOption func(*findOptions)

type findOptions struct {
	sort        []SortField
	limit, skip int64
}

// WithSort orders the results; earlier fields take precedence.
func WithSort(fields ...SortField) FindOption {
	return func(o *findOptions) { o.sort = append(o.sort, fields...) }
}

// WithLimit returns at most n results.
func WithLimit(n int64) FindOption { return func(o *findOptions) { o.limit = n } }

// WithSkip skips the first n results.
func WithSkip(n int64) FindOption { return func(o *findOptions) { o.skip = n } }

func resolveFindOptions(opts ...FindOption) (findOptions, error) {
	var o findOptions
	for _, opt := range opts {
		opt(&o)
	}
	if o.limit < 0 || o.skip < 0 {
		return o, fmt.Errorf("odm/mongodb: limit and skip must not be negative (limit %d, skip %d)", o.limit, o.skip)
	}
	for _, s := range o.sort {
		if s.Field == "" {
			return o, errors.New("odm/mongodb: sort field must not be empty")
		}
	}
	return o, nil
}

// VectorSearchParams configures VectorSearch.
type VectorSearchParams struct {
	IndexName     string // a VectorIndexSpec of the model
	K             int    // number of nearest neighbours to return
	NumCandidates int    // approximate candidates to consider; 0 picks a default
	Filter        bson.M // optional pre-filter on the index's FilterPaths
}

func (p VectorSearchParams) validate(embedding []float32) error {
	if len(embedding) == 0 || p.IndexName == "" || p.K <= 0 {
		return errors.New("odm/mongodb: VectorSearch needs an embedding, an index name and K > 0")
	}
	if p.NumCandidates < 0 {
		return errors.New("odm/mongodb: VectorSearch NumCandidates must not be negative")
	}
	return nil
}

// TermSearchParams configures TermSearch.
type TermSearchParams struct {
	IndexName string // a TermSearchIndexSpec of the model
	Limit     int    // number of results to return
	Filter    bson.M // optional filter
}

func (p TermSearchParams) validate(query string) error {
	if query == "" || p.IndexName == "" || p.Limit <= 0 {
		return errors.New("odm/mongodb: TermSearch needs a query, an index name and Limit > 0")
	}
	return nil
}

// DistinctAs is Distinct with values decoded into V, e.g.
// mongodb.DistinctAs[string](ctx, movies, "genres", nil).
func DistinctAs[V any, T odm.Model](ctx context.Context, c Collection[T], field string, filter bson.M) <-chan async.Result[[]V] {
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
			return nil, fmt.Errorf("odm/mongodb: decoding distinct values of %s: %w", field, err)
		}
		if out.V == nil {
			out.V = []V{}
		}
		return out.V, nil
	})
}
