// Package odm is a generic, multi-tenant object-document mapper with two
// backends: MongoDB (package odm/mongodb) and Postgres (package odm/postgres).
//
// This package holds what both backends share: the Model contract, the
// Collection interface, the filter, sort and index types, and the errors.
// Application code written against odm.Collection[T] runs on either backend.
//
// Filters use MongoDB's query syntax with this package's types:
//
//	odm.M{"year": odm.M{"$gte": 2020}, "genres": odm.M{"$in": odm.A{"drama", "crime"}}}
//
// Every IO method returns a channel carrying an async.Result, so independent
// queries run concurrently:
//
//	movie, err := async.Await(movies.FindByID(ctx, id))
//
// Documents are encoded with the model's bson struct tags on both backends.
package odm

import (
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jinzhu/copier"
	"golang.org/x/crypto/blake2s"
)

// Model is a document type stored in a collection.
//
// Implement both methods on the value receiver: collections are created from
// the zero value of T.
type Model interface {
	// Id returns the document's unique id. odm uses string ids derived from the
	// document's data; when several fields identify a document, combine them
	// with HashedKey.
	Id() string
	// CollectionName names the MongoDB collection or Postgres table.
	CollectionName() string
}

// NewModelFrom copies matching fields from src (typically a protobuf message)
// into a new T.
func NewModelFrom[T any](src any) *T {
	model := new(T)
	_ = copier.Copy(model, src)
	return model
}

// HashedKey derives a short, stable id from fields. Order matters.
func HashedKey(fields ...string) (string, error) {
	if len(fields) == 0 {
		return "", nil
	}

	h, err := blake2s.New256(nil)
	if err != nil {
		return "", err
	}
	if _, err = h.Write([]byte(strings.Join(fields, ">"))); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:12], nil
}

// CheckID rejects empty ids, which would silently share one document.
func CheckID(id string) error {
	if id == "" {
		return errors.New("odm: model Id() must not be empty")
	}
	return nil
}

// Timer supplies the createdOn / updatedOn timestamps, in Unix seconds.
type Timer interface {
	Now() int64
}

// SystemTimer is the wall-clock Timer.
type SystemTimer struct{}

func (SystemTimer) Now() int64 { return time.Now().Unix() }
