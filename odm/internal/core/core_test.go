package core

import (
	"errors"
	"testing"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHelpers(t *testing.T) {
	assert.ErrorIs(t, Unsupported("TTL on %s", "x"), errors.ErrUnsupported)
	assert.Error(t, CheckID(""))
	assert.NoError(t, CheckID("a"))
}

type indexed struct{}

func (indexed) Id() string             { return "" }
func (indexed) CollectionName() string { return "things" }
func (indexed) VectorIndexes() []odm.VectorIndexSpec {
	return []odm.VectorIndexSpec{{Name: "v", Path: "e", Dimensions: 3}}
}

func TestDeclaredIndexes(t *testing.T) {
	vectors := VectorIndexes[indexed]()
	require.Len(t, vectors, 1)
	assert.Equal(t, odm.Cosine, Similarity(vectors[0]))
	assert.Nil(t, TermSearchIndexes[indexed]())
	assert.Nil(t, Indexes[indexed]())
}

func TestIndexName(t *testing.T) {
	assert.Equal(t, "year_-1_title_1", IndexName(odm.IndexSpec{Keys: []odm.IndexKey{odm.DescKey("year"), odm.Key("title")}}))
	assert.Equal(t, "byYear", IndexName(odm.IndexSpec{Name: "byYear"}))
}

func TestValidation(t *testing.T) {
	assert.NoError(t, ValidateIndex(odm.IndexSpec{Keys: []odm.IndexKey{odm.Key("a")}}))
	assert.ErrorContains(t, ValidateIndex(odm.IndexSpec{Name: "x"}), "no keys")
	assert.ErrorContains(t, ValidateIndex(odm.IndexSpec{Keys: []odm.IndexKey{odm.Key("")}}), "empty key")

	assert.NoError(t, ValidateTermSearchIndex(odm.TermSearchIndexSpec{Name: "t", Paths: []string{"a"}}))
	assert.Error(t, ValidateTermSearchIndex(odm.TermSearchIndexSpec{Name: "t"}))
	assert.Error(t, ValidateTermSearchIndex(odm.TermSearchIndexSpec{Name: "t", Paths: []string{""}}))

	vec := odm.VectorIndexSpec{Name: "v", Path: "e", Dimensions: 2}
	assert.NoError(t, ValidateVectorIndex(vec))
	assert.Error(t, ValidateVectorIndex(odm.VectorIndexSpec{Name: "v", Path: "e"}))
	assert.ErrorContains(t, ValidateVectorIndex(odm.VectorIndexSpec{Name: "v", Path: "e", Dimensions: 2, Similarity: "manhattan"}), "unknown similarity")

	assert.NoError(t, CheckVectorQuery(vec, []float32{1, 2}, 1))
	assert.Error(t, CheckVectorQuery(vec, nil, 1))
	assert.Error(t, CheckVectorQuery(vec, []float32{1, 2}, 0))
	assert.ErrorContains(t, CheckVectorQuery(vec, []float32{1}, 1), "has 2 dimensions, query has 1")

	assert.NoError(t, CheckTermQuery("q", 1))
	assert.Error(t, CheckTermQuery("", 1))
	assert.Error(t, CheckTermQuery("q", 0))
}
