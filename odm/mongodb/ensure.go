package mongodb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// EnsureCollection creates T's collection in tenant and every index T
// declares. Run it when deploying or onboarding a tenant; it is idempotent.
//
//   - Regular indexes whose definition changed are dropped and recreated.
//   - Atlas Search and Vector Search indexes are created when missing. Atlas
//     builds them asynchronously; WaitForSearchIndexes waits until they serve
//     queries. Changing an existing search index's definition needs a new name
//     (or dropping it first).
func EnsureCollection[T odm.Model](ctx context.Context, c *Client, tenant string) error {
	var zero T
	name := zero.CollectionName()
	db := c.mongo.Database(tenant)

	if err := db.CreateCollection(ctx, name); err != nil {
		var cmdErr mongo.CommandError
		if !errors.As(err, &cmdErr) || cmdErr.Code != 48 { // 48 = NamespaceExists
			return err
		}
	}
	col := db.Collection(name)

	for _, spec := range odm.IndexesOf[T]() {
		if err := ensureIndex(ctx, col, spec); err != nil {
			return err
		}
	}

	searchModels := map[string]mongo.SearchIndexModel{}
	for _, spec := range odm.TermSearchIndexesOf[T]() {
		searchModels[spec.Name] = termSearchIndexModel(spec)
	}
	for _, spec := range odm.VectorIndexesOf[T]() {
		m, err := vectorIndexModel(spec)
		if err != nil {
			return err
		}
		searchModels[spec.Name] = m
	}
	if len(searchModels) == 0 {
		return nil
	}

	existing, err := searchIndexStatus(ctx, col)
	if err != nil {
		return err
	}
	var missing []mongo.SearchIndexModel
	for name, m := range searchModels {
		if _, ok := existing[name]; !ok {
			missing = append(missing, m)
		}
	}
	if len(missing) > 0 {
		if _, err := col.SearchIndexes().CreateMany(ctx, missing); err != nil {
			return err
		}
	}
	return nil
}

// WaitForSearchIndexes blocks until every Atlas Search / Vector Search index T
// declares in tenant can serve queries, or ctx ends.
func WaitForSearchIndexes[T odm.Model](ctx context.Context, c *Client, tenant string) error {
	var names []string
	for _, s := range odm.TermSearchIndexesOf[T]() {
		names = append(names, s.Name)
	}
	for _, s := range odm.VectorIndexesOf[T]() {
		names = append(names, s.Name)
	}
	var zero T
	col := c.collection(tenant, zero.CollectionName())

	for {
		status, err := searchIndexStatus(ctx, col)
		if err != nil {
			return err
		}
		ready := true
		for _, n := range names {
			if !status[n] {
				ready = false
			}
		}
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("odm/mongodb: waiting for search indexes on %s.%s: %w", tenant, zero.CollectionName(), ctx.Err())
		case <-time.After(time.Second):
		}
	}
}

// searchIndexStatus maps each search index name to whether it is queryable.
func searchIndexStatus(ctx context.Context, col *mongo.Collection) (map[string]bool, error) {
	cursor, err := col.SearchIndexes().List(ctx, nil)
	if err != nil {
		return nil, err
	}
	var indexes []struct {
		Name      string `bson:"name"`
		Queryable bool   `bson:"queryable"`
	}
	if err := cursor.All(ctx, &indexes); err != nil {
		return nil, err
	}
	status := make(map[string]bool, len(indexes))
	for _, ix := range indexes {
		status[ix.Name] = ix.Queryable
	}
	return status, nil
}

func ensureIndex(ctx context.Context, col *mongo.Collection, spec odm.IndexSpec) error {
	if len(spec.Keys) == 0 {
		return fmt.Errorf("odm/mongodb: index %q has no keys", spec.Name)
	}
	keys := bson.D{}
	for _, k := range spec.Keys {
		dir := 1
		if k.Desc {
			dir = -1
		}
		keys = append(keys, bson.E{Key: k.Field, Value: dir})
	}
	opts := options.Index().SetName(spec.DisplayName())
	if spec.Unique {
		opts.SetUnique(true)
	}
	if spec.TTL > 0 {
		opts.SetExpireAfterSeconds(int32(spec.TTL / time.Second))
	}
	model := mongo.IndexModel{Keys: keys, Options: opts}

	_, err := col.Indexes().CreateOne(ctx, model)
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) && (cmdErr.Code == 85 || cmdErr.Code == 86) { // IndexOptionsConflict, IndexKeySpecsConflict
		// The definition changed: replace the index.
		if err := col.Indexes().DropOne(ctx, spec.DisplayName()); err != nil {
			return err
		}
		_, err = col.Indexes().CreateOne(ctx, model)
	}
	return err
}

func termSearchIndexModel(spec odm.TermSearchIndexSpec) mongo.SearchIndexModel {
	// Dotted paths become nested document mappings.
	fields := bson.M{}
	for _, path := range spec.Paths {
		parts := strings.Split(path, ".")
		level := fields
		for _, p := range parts[:len(parts)-1] {
			next, ok := level[p].(bson.M)
			if !ok {
				next = bson.M{"type": "document", "fields": bson.M{}}
				level[p] = next
			}
			level = next["fields"].(bson.M)
		}
		level[parts[len(parts)-1]] = bson.M{"type": "string"}
	}
	return mongo.SearchIndexModel{
		Definition: bson.M{"mappings": bson.M{"dynamic": false, "fields": fields}},
		Options:    options.SearchIndexes().SetName(spec.Name).SetType("search"),
	}
}

func vectorIndexModel(spec odm.VectorIndexSpec) (mongo.SearchIndexModel, error) {
	if spec.Name == "" || spec.Path == "" || spec.Dimensions <= 0 {
		return mongo.SearchIndexModel{}, fmt.Errorf("odm/mongodb: vector index %q needs a name, path and dimensions", spec.Name)
	}
	vector := bson.D{
		{Key: "type", Value: "vector"},
		{Key: "path", Value: spec.Path},
		{Key: "numDimensions", Value: spec.Dimensions},
		{Key: "similarity", Value: string(spec.SimilarityOrDefault())},
	}
	if spec.Quantization != "" {
		vector = append(vector, bson.E{Key: "quantization", Value: spec.Quantization})
	}
	fields := bson.A{vector}
	for _, p := range spec.FilterPaths {
		fields = append(fields, bson.D{{Key: "type", Value: "filter"}, {Key: "path", Value: p}})
	}
	return mongo.SearchIndexModel{
		Definition: bson.D{{Key: "fields", Value: fields}},
		Options:    options.SearchIndexes().SetName(spec.Name).SetType("vectorSearch"),
	}, nil
}
