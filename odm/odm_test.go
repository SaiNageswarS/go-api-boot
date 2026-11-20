package odm

import (
	"testing"

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
