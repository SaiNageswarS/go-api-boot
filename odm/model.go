// Package odm holds what go-api-boot's two data backends share: the Model
// contract, index declarations, search hits and errors.
//
// Each backend has its own API, native to its database:
//
//   - odm/mongodb: collections queried with MongoDB filter documents (bson.M),
//     with Atlas Search and Vector Search;
//   - odm/postgres: typed tables queried with SQL conditions, with pgvector
//     and ParadeDB search.
//
// Every IO method in both returns a channel carrying an async.Result, so
// independent queries run concurrently:
//
//	movie, err := async.Await(movies.FindByID(ctx, id))
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
