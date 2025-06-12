package odm

import (
	"encoding/hex"
	"strings"

	"github.com/jinzhu/copier"
	"go.mongodb.org/mongo-driver/v2/bson"
	"golang.org/x/crypto/blake2s"
)

type DbModel interface {
	// odm enforces string ID instead of ObjectID.
	// Principally, odm encourages to use data properties to find unique ID for a document.
	// If multiple fields together can uniquely identify a document, then hash those fields into a single string.
	Id() string
	CollectionName() string
}

func NewModelFrom[T any](proto interface{}) *T {
	model := new(T)
	_ = copier.Copy(model, proto)
	return model
}

func HashedKey(fields ...string) (string, error) {
	if len(fields) == 0 {
		return "", nil // No fields to hash, return empty string
	}

	key := strings.Join(fields, ">")
	h, err := blake2s.New256(nil)
	if err != nil {
		return "", err
	}

	if _, err = h.Write([]byte(key)); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil))[:12], nil
}

type SearchHit[T DbModel] struct {
	Score float64 `bson:"score"`
	Doc   T       `bson:"doc"`
}

type VectorSearchParams struct {
	IndexName     string // Atlas Vector Search index name
	Path          string // field in the collection that holds the embedding (e.g. "embedding")
	K             int    // number of nearest neighbours
	NumCandidates int    // count of initial candidates to be considered for nearest search. These are approximate neighbours..
	Filter        bson.M // optional pre-filter; nil for none
}

type TermSearchParams struct {
	IndexName string   // required
	Path      []string // field to search
	Filter    bson.M   // optional filter
	Limit     int      // number of results to return
}
