package odm

import (
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type VectorIndexSpec struct {
	Name          string `bson:"-"`    //index name
	Type          string `bson:"type"` // field type, e.g. "vector"
	Path          string `bson:"path"` // e.g. field name in the struct/json that holds the embedding.
	NumDimensions int    `bson:"numDimensions"`
	Similarity    string `bson:"similarity,omitempty"` // e.g. "cosine", "dotProduct", "euclidean"
	Quantization  string `bson:"quantization,omitempty"`
}

func (v VectorIndexSpec) Model() mongo.SearchIndexModel {
	def := struct {
		Fields []VectorIndexSpec `bson:"fields"`
	}{
		Fields: []VectorIndexSpec{v},
	}

	opts := options.SearchIndexes().
		SetName(v.Name).
		SetType("vectorSearch")

	return mongo.SearchIndexModel{
		Definition: def,
		Options:    opts,
	}
}

type TermSearchIndexSpec struct {
	Name  string   // index name
	Paths []string // e.g. fields in the struct that holds the text to be indexed.
}

func (t TermSearchIndexSpec) Model() mongo.SearchIndexModel {
	fields := bson.D{}
	for _, path := range t.Paths {
		fields = append(fields, bson.E{Key: path, Value: bson.D{{Key: "type", Value: "string"}}})
	}

	def := bson.D{
		{Key: "mappings", Value: bson.D{
			{Key: "dynamic", Value: false},
			{Key: "fields", Value: fields},
		}},
	}

	opts := options.SearchIndexes().
		SetName(t.Name).
		SetType("search")

	return mongo.SearchIndexModel{
		Definition: def,
		Options:    opts,
	}
}
