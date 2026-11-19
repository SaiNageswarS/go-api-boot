package mongodb

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-api-boot/odm/internal/asyncx"
	"github.com/SaiNageswarS/go-collection-boot/async"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Collection is a MongoDB collection: the portable odm.Collection plus
// aggregation pipelines and the raw driver collection.
type Collection[T odm.Model] interface {
	odm.Collection[T]

	// Aggregate runs pipeline and decodes each output document into T.
	Aggregate(ctx context.Context, pipeline mongo.Pipeline) <-chan async.Result[[]T]
	// Raw returns the driver collection, for anything the ODM doesn't cover.
	Raw() *mongo.Collection
}

var _ Collection[odm.Model] = (*collection[odm.Model])(nil)

type collection[T odm.Model] struct {
	col   *mongo.Collection
	timer odm.Timer
}

// CollectionOf returns the collection of T in tenant (a MongoDB database).
func CollectionOf[T odm.Model](c *Client, tenant string) Collection[T] {
	var zero T
	return &collection[T]{col: c.collection(tenant, zero.CollectionName()), timer: c.timer}
}

func (c *collection[T]) Raw() *mongo.Collection { return c.col }

// Save upserts in one atomic update: model's fields and updatedOn are $set,
// createdOn is set only on insert.
func (c *collection[T]) Save(ctx context.Context, model T) <-chan async.Result[struct{}] {
	return asyncx.Go(func() (struct{}, error) {
		id := model.Id()
		if err := odm.CheckID(id); err != nil {
			return struct{}{}, err
		}

		raw, err := bson.Marshal(model)
		if err != nil {
			return struct{}{}, err
		}
		var doc bson.M
		if err := bson.Unmarshal(raw, &doc); err != nil {
			return struct{}{}, err
		}
		delete(doc, "_id")
		delete(doc, "createdOn")
		now := c.timer.Now()
		doc["updatedOn"] = now

		_, err = c.col.UpdateOne(ctx,
			bson.M{"_id": id},
			bson.M{"$set": doc, "$setOnInsert": bson.M{"createdOn": now}},
			options.UpdateOne().SetUpsert(true))
		return struct{}{}, mapErr(err)
	})
}

func (c *collection[T]) FindByID(ctx context.Context, id string) <-chan async.Result[*T] {
	return c.FindOne(ctx, odm.M{"_id": id})
}

func (c *collection[T]) FindOne(ctx context.Context, filter odm.M) <-chan async.Result[*T] {
	return asyncx.Go(func() (*T, error) {
		res := c.col.FindOne(ctx, filterDoc(filter))
		if err := res.Err(); err != nil {
			return nil, mapErr(err)
		}
		model := new(T)
		return model, res.Decode(model)
	})
}

func (c *collection[T]) Find(ctx context.Context, filter odm.M, opts ...odm.FindOption) <-chan async.Result[[]T] {
	return asyncx.Go(func() ([]T, error) {
		o, err := odm.ResolveFindOptions(opts...)
		if err != nil {
			return nil, err
		}
		findOpts := options.Find().SetSkip(o.Skip)
		if o.Limit > 0 {
			findOpts.SetLimit(o.Limit)
		}
		if len(o.Sort) > 0 {
			findOpts.SetSort(sortDoc(o.Sort))
		}
		cursor, err := c.col.Find(ctx, filterDoc(filter), findOpts)
		if err != nil {
			return nil, err
		}
		result := []T{}
		return result, cursor.All(ctx, &result)
	})
}

func (c *collection[T]) Count(ctx context.Context, filter odm.M) <-chan async.Result[int64] {
	return asyncx.Go(func() (int64, error) {
		return c.col.CountDocuments(ctx, filterDoc(filter))
	})
}

func (c *collection[T]) Exists(ctx context.Context, id string) <-chan async.Result[bool] {
	return asyncx.Go(func() (bool, error) {
		n, err := c.col.CountDocuments(ctx, bson.M{"_id": id}, options.Count().SetLimit(1))
		return n > 0, err
	})
}

func (c *collection[T]) Distinct(ctx context.Context, field string, filter odm.M) <-chan async.Result[[]any] {
	return asyncx.Go(func() ([]any, error) {
		res := c.col.Distinct(ctx, field, filterDoc(filter))
		if err := res.Err(); err != nil {
			return nil, err
		}
		values := []any{}
		return values, res.Decode(&values)
	})
}

func (c *collection[T]) DeleteByID(ctx context.Context, id string) <-chan async.Result[struct{}] {
	return c.DeleteOne(ctx, odm.M{"_id": id})
}

func (c *collection[T]) DeleteOne(ctx context.Context, filter odm.M) <-chan async.Result[struct{}] {
	return asyncx.Go(func() (struct{}, error) {
		if filter == nil {
			return struct{}{}, errors.New("odm/mongodb: DeleteOne needs a filter")
		}
		_, err := c.col.DeleteOne(ctx, filterDoc(filter))
		return struct{}{}, err
	})
}

func (c *collection[T]) Aggregate(ctx context.Context, pipeline mongo.Pipeline) <-chan async.Result[[]T] {
	return asyncx.Go(func() ([]T, error) {
		cursor, err := c.col.Aggregate(ctx, pipeline)
		if err != nil {
			return nil, err
		}
		result := []T{}
		return result, cursor.All(ctx, &result)
	})
}

// VectorSearch runs Atlas Vector Search ($vectorSearch).
func (c *collection[T]) VectorSearch(ctx context.Context, embedding []float32, params odm.VectorSearchParams) <-chan async.Result[[]odm.SearchHit[T]] {
	return asyncx.Go(func() ([]odm.SearchHit[T], error) {
		if err := params.Validate(embedding); err != nil {
			return nil, err
		}
		spec, err := odm.VectorIndexOf[T](params.IndexName)
		if err != nil {
			return nil, err
		}
		if spec.Dimensions != len(embedding) {
			return nil, fmt.Errorf("odm/mongodb: index %s has %d dimensions, query has %d", spec.Name, spec.Dimensions, len(embedding))
		}

		numCandidates := params.NumCandidates
		if numCandidates == 0 {
			numCandidates = min(max(params.K*10, 100), 10000)
		}
		stage := bson.D{
			{Key: "index", Value: params.IndexName},
			{Key: "path", Value: spec.Path},
			{Key: "queryVector", Value: bson.NewVector(embedding).Binary()},
			{Key: "numCandidates", Value: max(numCandidates, params.K)},
			{Key: "limit", Value: params.K},
		}
		if len(params.Filter) > 0 {
			stage = append(stage, bson.E{Key: "filter", Value: filterDoc(params.Filter)})
		}
		return c.search(ctx, mongo.Pipeline{
			{{Key: "$vectorSearch", Value: stage}},
			{{Key: "$project", Value: bson.D{
				{Key: "score", Value: bson.D{{Key: "$meta", Value: "vectorSearchScore"}}},
				{Key: "doc", Value: "$$ROOT"},
			}}},
		})
	})
}

// TermSearch runs Atlas Search ($search) over the index's paths.
func (c *collection[T]) TermSearch(ctx context.Context, query string, params odm.TermSearchParams) <-chan async.Result[[]odm.SearchHit[T]] {
	return asyncx.Go(func() ([]odm.SearchHit[T], error) {
		if err := params.Validate(query); err != nil {
			return nil, err
		}
		spec, err := odm.TermSearchIndexOf[T](params.IndexName)
		if err != nil {
			return nil, err
		}

		pipeline := mongo.Pipeline{
			{{Key: "$search", Value: bson.D{
				{Key: "index", Value: params.IndexName},
				{Key: "text", Value: bson.D{{Key: "query", Value: query}, {Key: "path", Value: spec.Paths}}},
			}}},
		}
		if len(params.Filter) > 0 {
			pipeline = append(pipeline, bson.D{{Key: "$match", Value: filterDoc(params.Filter)}})
		}
		pipeline = append(pipeline,
			bson.D{{Key: "$limit", Value: params.Limit}},
			bson.D{{Key: "$project", Value: bson.D{
				{Key: "score", Value: bson.D{{Key: "$meta", Value: "searchScore"}}},
				{Key: "doc", Value: "$$ROOT"},
			}}},
		)
		return c.search(ctx, pipeline)
	})
}

func (c *collection[T]) search(ctx context.Context, pipeline mongo.Pipeline) ([]odm.SearchHit[T], error) {
	cursor, err := c.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	hits := []odm.SearchHit[T]{}
	return hits, cursor.All(ctx, &hits)
}

func filterDoc(filter odm.M) any {
	if filter == nil {
		return bson.M{}
	}
	return filter
}

func sortDoc(fields []odm.SortField) bson.D {
	d := make(bson.D, len(fields))
	for i, f := range fields {
		dir := 1
		if f.Desc {
			dir = -1
		}
		d[i] = bson.E{Key: f.Field, Value: dir}
	}
	return d
}

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
