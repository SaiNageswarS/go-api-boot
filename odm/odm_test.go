package odm

import (
	"context"
	"testing"

	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashedKey(t *testing.T) {
	a, err := HashedKey("x", "y")
	require.NoError(t, err)
	b, _ := HashedKey("x", "y")
	c, _ := HashedKey("y", "x")
	assert.Equal(t, a, b, "deterministic")
	assert.NotEqual(t, a, c, "order matters")
	assert.Len(t, a, 12)

	empty, err := HashedKey()
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestNewModelFrom(t *testing.T) {
	type src struct{ Name string }
	type dst struct{ Name string }
	assert.Equal(t, "n", NewModelFrom[dst](src{Name: "n"}).Name)
}

func TestResolveFindOptions(t *testing.T) {
	o, err := ResolveFindOptions(WithSort(Desc("year"), Asc("title")), WithLimit(5), WithSkip(2))
	require.NoError(t, err)
	assert.Equal(t, FindOptions{Sort: []SortField{{"year", true}, {"title", false}}, Limit: 5, Skip: 2}, o)

	_, err = ResolveFindOptions(WithSkip(-1))
	assert.Error(t, err)
	_, err = ResolveFindOptions(WithSort(Asc("")))
	assert.Error(t, err)
}

func TestIndexSpecDisplayName(t *testing.T) {
	assert.Equal(t, "year_-1_title_1", IndexSpec{Keys: []IndexKey{DescKey("year"), Key("title")}}.DisplayName())
	assert.Equal(t, "byYear", IndexSpec{Name: "byYear"}.DisplayName())
}

type indexed struct{}

func (indexed) Id() string             { return "" }
func (indexed) CollectionName() string { return "things" }
func (indexed) VectorIndexes() []VectorIndexSpec {
	return []VectorIndexSpec{{Name: "v", Path: "e", Dimensions: 3}}
}

func TestIndexLookup(t *testing.T) {
	spec, err := VectorIndexOf[indexed]("v")
	require.NoError(t, err)
	assert.Equal(t, Cosine, spec.SimilarityOrDefault())

	_, err = VectorIndexOf[indexed]("nope")
	assert.ErrorContains(t, err, `things declares no vector index "nope"`)
	_, err = TermSearchIndexOf[indexed]("t")
	assert.Error(t, err)
}

func TestSearchParamsValidate(t *testing.T) {
	assert.NoError(t, VectorSearchParams{IndexName: "v", K: 1}.Validate([]float32{1}))
	assert.Error(t, VectorSearchParams{IndexName: "v"}.Validate([]float32{1}))
	assert.Error(t, VectorSearchParams{IndexName: "v", K: 1}.Validate(nil))
	assert.NoError(t, TermSearchParams{IndexName: "t", Limit: 1}.Validate("q"))
	assert.Error(t, TermSearchParams{IndexName: "t", Limit: 1}.Validate(""))
}

// fakeDistinct implements just Distinct, for DistinctAs.
type fakeDistinct struct {
	Collection[indexed]
	values []any
}

func (f fakeDistinct) Distinct(context.Context, string, M) <-chan async.Result[[]any] {
	return async.Go(func() ([]any, error) { return f.values, nil })
}

func TestDistinctAs(t *testing.T) {
	years, err := async.Await(DistinctAs[int](t.Context(), Collection[indexed](fakeDistinct{values: []any{int32(1995), int64(1998)}}), "year", nil))
	require.NoError(t, err)
	assert.Equal(t, []int{1995, 1998}, years)

	none, err := async.Await(DistinctAs[string](t.Context(), Collection[indexed](fakeDistinct{values: []any{}}), "x", nil))
	require.NoError(t, err)
	assert.Equal(t, []string{}, none)
}
