package mongodb

import (
	"errors"
	"fmt"

	"github.com/SaiNageswarS/go-api-boot/odm/internal/core"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// The options and parameters of Collection's methods, and their driver form.

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

// VectorSearchParams configures VectorSearch.
type VectorSearchParams struct {
	K             int    // number of nearest neighbours to return
	NumCandidates int    // approximate candidates to consider; 0 picks a default
	Filter        bson.M // optional pre-filter on the index's FilterPaths
}

// TermSearchParams configures TermSearch.
type TermSearchParams struct {
	Limit  int    // number of results to return
	Filter bson.M // optional filter
}

func resolveFindOptions(opts ...FindOption) (findOptions, error) {
	var o findOptions
	for _, opt := range opts {
		opt(&o)
	}
	if o.limit < 0 || o.skip < 0 {
		return o, fmt.Errorf("limit and skip must not be negative (limit %d, skip %d)", o.limit, o.skip)
	}
	for _, s := range o.sort {
		if s.Field == "" {
			return o, errors.New("sort field must not be empty")
		}
	}
	return o, nil
}

func (o findOptions) driverOptions() *options.FindOptionsBuilder {
	opts := options.Find().SetSkip(o.skip)
	if o.limit > 0 {
		opts.SetLimit(o.limit)
	}
	if len(o.sort) > 0 {
		opts.SetSort(sortDoc(o.sort))
	}
	return opts
}

func sortDoc(fields []SortField) bson.D {
	d := make(bson.D, len(fields))
	for i, f := range fields {
		d[i] = bson.E{Key: f.Field, Value: core.Direction(f.Desc)}
	}
	return d
}

// filterDoc is filter, or the match-everything document for nil.
func filterDoc(filter bson.M) bson.M {
	if filter == nil {
		return bson.M{}
	}
	return filter
}
