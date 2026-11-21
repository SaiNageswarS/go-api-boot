package mongodb

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-api-boot/odm/internal/core"
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
func EnsureCollection[T odm.Model](ctx context.Context, c *Client, tenant string) (err error) {
	var zero T
	name := zero.CollectionName()
	defer annotate(&err, tenant+"."+name)

	// Build every definition first, so an invalid spec changes nothing.
	var indexes []odm.IndexSpec
	for _, spec := range core.Indexes[T]() {
		if err := core.ValidateIndex(spec); err != nil {
			return err
		}
		indexes = append(indexes, spec)
	}
	searchModels := map[string]mongo.SearchIndexModel{}
	for _, spec := range core.TermSearchIndexes[T]() {
		m, err := termSearchIndexModel(spec)
		if err != nil {
			return err
		}
		searchModels[spec.Name] = m
	}
	for _, spec := range core.VectorIndexes[T]() {
		m, err := vectorIndexModel(spec)
		if err != nil {
			return err
		}
		searchModels[spec.Name] = m
	}

	db := c.mongo.Database(tenant)
	if err := db.CreateCollection(ctx, name); err != nil {
		var cmdErr mongo.CommandError
		if !errors.As(err, &cmdErr) || cmdErr.Code != 48 { // 48 = NamespaceExists
			return err
		}
	}
	col := db.Collection(name)

	for _, spec := range indexes {
		if err := ensureIndex(ctx, col, spec); err != nil {
			return err
		}
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
func WaitForSearchIndexes[T odm.Model](ctx context.Context, c *Client, tenant string) (err error) {
	var zero T
	name := zero.CollectionName()
	defer annotate(&err, tenant+"."+name)

	var names []string
	for _, s := range core.TermSearchIndexes[T]() {
		names = append(names, s.Name)
	}
	for _, s := range core.VectorIndexes[T]() {
		names = append(names, s.Name)
	}
	col := c.collection(tenant, name)

	for {
		status, err := searchIndexStatus(ctx, col)
		if err != nil {
			return err
		}
		ready := true
		for _, n := range names {
			ready = ready && status[n]
		}
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
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

// ensureIndex creates a regular index, replacing one of the same name whose
// definition changed.
func ensureIndex(ctx context.Context, col *mongo.Collection, spec odm.IndexSpec) error {
	keys := make(bson.D, len(spec.Keys))
	for i, k := range spec.Keys {
		keys[i] = bson.E{Key: k.Field, Value: core.Direction(k.Desc)}
	}
	name := core.IndexName(spec)
	opts := options.Index().SetName(name)
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
		if err := col.Indexes().DropOne(ctx, name); err != nil {
			return err
		}
		_, err = col.Indexes().CreateOne(ctx, model)
	}
	return err
}

func termSearchIndexModel(spec odm.TermSearchIndexSpec) (mongo.SearchIndexModel, error) {
	if err := core.ValidateTermSearchIndex(spec); err != nil {
		return mongo.SearchIndexModel{}, err
	}
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
	}, nil
}

func vectorIndexModel(spec odm.VectorIndexSpec) (mongo.SearchIndexModel, error) {
	if err := core.ValidateVectorIndex(spec); err != nil {
		return mongo.SearchIndexModel{}, err
	}
	vector := bson.D{
		{Key: "type", Value: "vector"},
		{Key: "path", Value: spec.Path},
		{Key: "numDimensions", Value: spec.Dimensions},
		{Key: "similarity", Value: string(core.Similarity(spec))},
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
