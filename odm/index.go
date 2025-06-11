package odm

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Classic B-tree / hashed / compound indexes.
type Indexed interface {
	IndexModels() []mongo.IndexModel
}

type SearchIndexed interface {
	TermSearchIndexSpecs() []TermSearchIndexSpec
}

type VectorIndexed interface {
	VectorIndexSpecs() []VectorIndexSpec
}

// EnsureIndexes creates every index the model advertises.
func EnsureIndexes[T DbModel](
	ctx context.Context,
	client MongoClient,
	tenant string,
) error {

	var zero T
	db := client.Database(tenant)
	collName := zero.CollectionName()

	// 1. Ensure the collection exists (safe to run more than once).
	if err := db.CreateCollection(ctx, collName); err != nil {
		var cmdErr mongo.CommandError
		if !errors.As(err, &cmdErr) || cmdErr.Code != 48 { // 48 = NamespaceExists
			return err // propagate any other failure
		}
		// Otherwise collection already exists – nothing to do.
	}

	coll := db.Collection(collName)

	// --- Classic indexes ----------------------------------------------------
	if ix, ok := any(zero).(Indexed); ok {
		if _, err := coll.Indexes().
			CreateMany(ctx, ix.IndexModels()); err != nil {
			return err
		}
	}

	// --- Atlas Search indexes ----------------------------------------------
	if sx, ok := any(zero).(SearchIndexed); ok {
		var models []mongo.SearchIndexModel
		for _, spec := range sx.TermSearchIndexSpecs() {
			models = append(models, spec.Model())
		}
		if _, err := coll.SearchIndexes().CreateMany(ctx, models); err != nil {
			return err
		}
	}

	// --- Atlas Vector indexes ----------------------------------------------
	if vx, ok := any(zero).(VectorIndexed); ok {
		var models []mongo.SearchIndexModel
		for _, spec := range vx.VectorIndexSpecs() {
			models = append(models, spec.Model())
		}
		if _, err := coll.SearchIndexes().CreateMany(ctx, models); err != nil {
			return err
		}
	}

	return nil
}
