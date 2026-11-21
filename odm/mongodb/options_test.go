package mongodb

import (
	"context"
	"testing"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Unit tests: no database needed.

func TestResolveFindOptions(t *testing.T) {
	o, err := resolveFindOptions(WithSort(Desc("year"), Asc("title")), WithLimit(5), WithSkip(2))
	require.NoError(t, err)
	assert.Equal(t, findOptions{sort: []SortField{{"year", true}, {"title", false}}, limit: 5, skip: 2}, o)
	assert.Equal(t, bson.D{{Key: "year", Value: -1}, {Key: "title", Value: 1}}, sortDoc(o.sort))

	_, err = resolveFindOptions(WithSkip(-1))
	assert.Error(t, err)
	_, err = resolveFindOptions(WithSort(Asc("")))
	assert.Error(t, err)
}

func TestSearchPipelines(t *testing.T) {
	vec := odm.VectorIndexSpec{Name: "v", Path: "e", Dimensions: 1}
	p, err := vectorSearchPipeline(vec, []float32{1}, VectorSearchParams{IndexName: "v", K: 3})
	require.NoError(t, err)
	stage := p[0][0].Value.(bson.D)
	assert.Contains(t, stage, bson.E{Key: "numCandidates", Value: 100}, "default candidates")
	_, err = vectorSearchPipeline(vec, []float32{1}, VectorSearchParams{IndexName: "v"})
	assert.Error(t, err, "K is required")
	_, err = vectorSearchPipeline(vec, []float32{1}, VectorSearchParams{IndexName: "v", K: 1, NumCandidates: -1})
	assert.Error(t, err)

	term := odm.TermSearchIndexSpec{Name: "t", Paths: []string{"plot"}}
	p, err = termSearchPipeline(term, "q", TermSearchParams{IndexName: "t", Limit: 1, Filter: bson.M{"year": 1}})
	require.NoError(t, err)
	assert.Len(t, p, 4, "search, match, limit, project")
	_, err = termSearchPipeline(term, "", TermSearchParams{IndexName: "t", Limit: 1})
	assert.Error(t, err)
}

// fakeDistinct implements just Distinct, for DistinctAs.
type fakeDistinct struct {
	Collection[Movie]
	values []any
}

func (f fakeDistinct) Distinct(context.Context, string, bson.M) <-chan async.Result[[]any] {
	return async.Go(func() ([]any, error) { return f.values, nil })
}

func TestDistinctAs(t *testing.T) {
	years, err := async.Await(DistinctAs[int](t.Context(), Collection[Movie](fakeDistinct{values: []any{int32(1995), int64(1998)}}), "year", nil))
	require.NoError(t, err)
	assert.Equal(t, []int{1995, 1998}, years)

	none, err := async.Await(DistinctAs[string](t.Context(), Collection[Movie](fakeDistinct{values: []any{}}), "x", nil))
	require.NoError(t, err)
	assert.Equal(t, []string{}, none)
}
