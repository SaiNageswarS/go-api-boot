package odm

import "fmt"

// M is a filter document in MongoDB query syntax, e.g.
// odm.M{"year": odm.M{"$gte": 2020}}. Dotted keys address nested fields.
type M map[string]any

// A is an array inside a filter, e.g. the operand of $in, $nin, $all, $or.
type A []any

// SortField orders results by one field.
type SortField struct {
	Field string
	Desc  bool
}

// Asc sorts by field, smallest first. Missing values sort first.
func Asc(field string) SortField { return SortField{Field: field} }

// Desc sorts by field, largest first. Missing values sort last.
func Desc(field string) SortField { return SortField{Field: field, Desc: true} }

// FindOption configures Find.
type FindOption func(*FindOptions)

// FindOptions is the resolved configuration of a Find call.
type FindOptions struct {
	Sort  []SortField
	Limit int64 // 0 means no limit
	Skip  int64
}

// WithSort orders the results; earlier fields take precedence.
func WithSort(fields ...SortField) FindOption {
	return func(o *FindOptions) { o.Sort = append(o.Sort, fields...) }
}

// WithLimit returns at most n results.
func WithLimit(n int64) FindOption { return func(o *FindOptions) { o.Limit = n } }

// WithSkip skips the first n results.
func WithSkip(n int64) FindOption { return func(o *FindOptions) { o.Skip = n } }

// ResolveFindOptions applies opts and validates the result. Backends call it.
func ResolveFindOptions(opts ...FindOption) (FindOptions, error) {
	var o FindOptions
	for _, opt := range opts {
		opt(&o)
	}
	if o.Limit < 0 || o.Skip < 0 {
		return o, fmt.Errorf("odm: limit and skip must not be negative (limit %d, skip %d)", o.Limit, o.Skip)
	}
	for _, s := range o.Sort {
		if s.Field == "" {
			return o, fmt.Errorf("odm: sort field must not be empty")
		}
	}
	return o, nil
}
