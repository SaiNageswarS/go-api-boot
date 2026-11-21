package mongodb

import (
	"errors"
	"fmt"
	"strings"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// annotate maps a non-nil *err to the odm errors and prefixes it with the
// collection it happened on: "odm/mongodb: <tenant>.<collection>: <cause>".
// Public functions defer it.
func annotate(err *error, target string) {
	if *err != nil {
		*err = fmt.Errorf("odm/mongodb: %s: %w", target, mapErr(*err))
	}
}

// mapErr turns the driver's not-found and duplicate key errors into the odm
// ones.
func mapErr(err error) error {
	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		return odm.ErrNotFound
	case mongo.IsDuplicateKeyError(err):
		return fmt.Errorf("%w: %s", odm.ErrDuplicateKey, strings.TrimSpace(err.Error()))
	}
	return err
}
