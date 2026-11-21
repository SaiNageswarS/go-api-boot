package odm

import (
	"encoding/hex"
	"strings"
	"time"

	"github.com/jinzhu/copier"
	"golang.org/x/crypto/blake2s"
)

// Model is a document type stored in a collection (MongoDB) or table
// (Postgres).
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

// Timestamps are the creation and last-save times, in Unix seconds, that Save
// writes on both backends. Save always writes them, whether or not the model
// embeds Timestamps, and ignores the values the model carries: CreatedOn is
// set on the first save and kept, UpdatedOn is set on every save.
//
// Embed Timestamps to read them back. MongoDB needs the inline tag:
//
//	type Profile struct {
//	    odm.Timestamps `bson:",inline"`
//	    ID   string    `bson:"_id" db:"id"`
//	    Name string    `bson:"name"`
//	}
type Timestamps struct {
	CreatedOn int64 `bson:"createdOn" db:"created_on" json:"createdOn"`
	UpdatedOn int64 `bson:"updatedOn" db:"updated_on" json:"updatedOn"`
}

// Timer supplies Timestamps, in Unix seconds.
type Timer interface {
	Now() int64
}

// SystemTimer is the wall-clock Timer.
type SystemTimer struct{}

func (SystemTimer) Now() int64 { return time.Now().Unix() }

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
