package mongodb

import (
	"errors"
	"fmt"
	"strings"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-api-boot/odm/internal/core"
	"github.com/SaiNageswarS/go-collection-boot/async"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const backend = "mongodb"

// run runs a collection operation asynchronously. Its error is mapped to the
// odm errors and wrapped as "odm/mongodb: <op> <target>: <cause>".
func run[R any](target, op string, fn func() (R, error)) <-chan async.Result[R] {
	return core.Go(backend, op, target, func() (R, error) {
		r, err := fn()
		return r, mapErr(err)
	})
}

// mapErr turns the driver's not-found and duplicate key errors into the odm
// ones.
func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, mongo.ErrNoDocuments):
		return odm.ErrNotFound
	case mongo.IsDuplicateKeyError(err):
		return fmt.Errorf("%w: %s", odm.ErrDuplicateKey, strings.TrimSpace(err.Error()))
	}
	return err
}
