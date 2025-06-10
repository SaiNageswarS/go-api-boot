package odm

import (
	"github.com/jinzhu/copier"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type DbModel interface {
	Id() string
	CollectionName() string
}

func NewModelFrom[T any](proto interface{}) *T {
	model := new(T)
	_ = copier.Copy(model, proto)
	return model
}

type SearchHit[T DbModel] struct {
	Score float64 `bson:"score"`
	Doc   T       `bson:"doc,inline"`
}

type VectorQuery struct {
	IndexName     string // Atlas Vector Search index name
	Path          string // field in the collection that holds the embedding (e.g. "embedding")
	K             int    // number of nearest neighbours
	NumCandidates int    // count of initial candidates to be considered for nearest search. These are approximate neighbours..
	Filter        bson.M // optional pre-filter; nil for none
}
