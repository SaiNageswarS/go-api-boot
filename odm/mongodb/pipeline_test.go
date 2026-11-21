package mongodb

import (
	"testing"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Unit tests: no database needed.

func TestSearchPipelines(t *testing.T) {
	vec := odm.VectorIndexSpec{Name: "v", Path: "e", Dimensions: 1}
	p, err := vectorSearchPipeline(vec, []float32{1}, VectorSearchParams{K: 3})
	require.NoError(t, err)
	stage := p[0][0].Value.(bson.D)
	assert.Contains(t, stage, bson.E{Key: "numCandidates", Value: 100}, "default candidates")
	_, err = vectorSearchPipeline(vec, []float32{1}, VectorSearchParams{})
	assert.Error(t, err, "K is required")
	_, err = vectorSearchPipeline(vec, []float32{1}, VectorSearchParams{K: 1, NumCandidates: -1})
	assert.Error(t, err)

	term := odm.TermSearchIndexSpec{Name: "t", Paths: []string{"plot"}}
	p, err = termSearchPipeline(term, "q", TermSearchParams{Limit: 1, Filter: bson.M{"year": 1}})
	require.NoError(t, err)
	assert.Len(t, p, 4, "search, match, limit, project")
	_, err = termSearchPipeline(term, "", TermSearchParams{Limit: 1})
	assert.Error(t, err)
}

func TestSearchPipelinesValidateTheIndex(t *testing.T) {
	_, err := vectorSearchPipeline(odm.VectorIndexSpec{Name: "v"}, []float32{1}, VectorSearchParams{K: 1})
	assert.ErrorContains(t, err, "needs a name, a path and dimensions")
	_, err = termSearchPipeline(odm.TermSearchIndexSpec{Name: "t"}, "q", TermSearchParams{Limit: 1})
	assert.ErrorContains(t, err, "needs a name and non-empty paths")
}
