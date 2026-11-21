package mongodb

import (
	"errors"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-api-boot/odm/internal/core"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// VectorSearchParams configures VectorSearch.
type VectorSearchParams struct {
	IndexName     string // a VectorIndexSpec of the model
	K             int    // number of nearest neighbours to return
	NumCandidates int    // approximate candidates to consider; 0 picks a default
	Filter        bson.M // optional pre-filter on the index's FilterPaths
}

// TermSearchParams configures TermSearch.
type TermSearchParams struct {
	IndexName string // a TermSearchIndexSpec of the model
	Limit     int    // number of results to return
	Filter    bson.M // optional filter
}

// vectorSearchPipeline is $vectorSearch, then the score and document as a
// SearchHit.
func vectorSearchPipeline(spec odm.VectorIndexSpec, embedding []float32, p VectorSearchParams) (mongo.Pipeline, error) {
	if err := core.CheckVectorQuery(spec, embedding, p.K); err != nil {
		return nil, err
	}
	if p.NumCandidates < 0 {
		return nil, errors.New("NumCandidates must not be negative")
	}
	numCandidates := p.NumCandidates
	if numCandidates == 0 {
		numCandidates = min(max(p.K*10, 100), 10000)
	}

	stage := bson.D{
		{Key: "index", Value: spec.Name},
		{Key: "path", Value: spec.Path},
		{Key: "queryVector", Value: bson.NewVector(embedding).Binary()},
		{Key: "numCandidates", Value: max(numCandidates, p.K)},
		{Key: "limit", Value: p.K},
	}
	if len(p.Filter) > 0 {
		stage = append(stage, bson.E{Key: "filter", Value: p.Filter})
	}
	return mongo.Pipeline{
		{{Key: "$vectorSearch", Value: stage}},
		hitProjection("vectorSearchScore"),
	}, nil
}

// termSearchPipeline is $search over the index's paths, the filter, the
// limit, then the score and document as a SearchHit.
func termSearchPipeline(spec odm.TermSearchIndexSpec, query string, p TermSearchParams) (mongo.Pipeline, error) {
	if err := core.CheckTermQuery(query, p.Limit); err != nil {
		return nil, err
	}
	pipeline := mongo.Pipeline{
		{{Key: "$search", Value: bson.D{
			{Key: "index", Value: spec.Name},
			{Key: "text", Value: bson.D{{Key: "query", Value: query}, {Key: "path", Value: spec.Paths}}},
		}}},
	}
	if len(p.Filter) > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: p.Filter}})
	}
	return append(pipeline,
		bson.D{{Key: "$limit", Value: p.Limit}},
		hitProjection("searchScore"),
	), nil
}

// hitProjection shapes each result as a SearchHit with the given $meta score.
func hitProjection(score string) bson.D {
	return bson.D{{Key: "$project", Value: bson.D{
		{Key: "score", Value: bson.D{{Key: "$meta", Value: score}}},
		{Key: "doc", Value: "$$ROOT"},
	}}}
}
