package odm

import (
	"context"
	"errors"

	"github.com/SaiNageswarS/go-api-boot/async"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type OdmCollectionInterface[T DbModel] interface {
	Save(ctx context.Context, model T) <-chan async.Result[struct{}]
	FindOneByID(ctx context.Context, id string) <-chan async.Result[*T]
	FindOne(ctx context.Context, filters bson.M) <-chan async.Result[*T]
	Find(ctx context.Context, filters bson.M, sort bson.D, limit, skip int64) <-chan async.Result[[]T]
	DeleteByID(ctx context.Context, id string) <-chan async.Result[struct{}]
	DeleteOne(ctx context.Context, filters bson.M) <-chan async.Result[struct{}]
	Count(ctx context.Context, filters bson.M) <-chan async.Result[int64]
	DistinctInto(ctx context.Context, field string, filters bson.D, out any) error
	Aggregate(ctx context.Context, pipeline mongo.Pipeline) <-chan async.Result[[]T]
	Exists(ctx context.Context, id string) <-chan async.Result[bool]
	VectorSearch(ctx context.Context, embedding []float32, opts VectorSearchParams) <-chan async.Result[[]SearchHit[T]]
	TermSearch(ctx context.Context, query string, params TermSearchParams) <-chan async.Result[[]SearchHit[T]]
}

type odmCollection[T DbModel] struct {
	col   CollectionInterface
	timer Timer
}

func CollectionOf[T DbModel](client MongoClient, tenant string) OdmCollectionInterface[T] {
	var zero T
	collName := zero.CollectionName()
	return &odmCollection[T]{
		col:   client.Database(tenant).Collection(collName),
		timer: DefaultTimer{},
	}
}

// Intentionally takes model value T. Avoid passing pointer to prevent
// accidental dereferencing of nil pointer.
// Also, passing pointer can fail CollectionName() in CollectionOf[T].
// Example usage:
// lead := db.LeadModel { Name: "Lead1" }
// _, err := async.Await(odm.CollectionOf[db.LeadModel](s.mongo, tenant).Save(ctx, lead))
func (c *odmCollection[T]) Save(ctx context.Context, model T) <-chan async.Result[struct{}] {
	return async.Go(func() (struct{}, error) {
		doc, err := convertToBson(model)
		if err != nil {
			return struct{}{}, err
		}

		doc["_id"] = model.Id()
		exists, _ := async.Await(c.Exists(ctx, model.Id()))
		if exists {
			doc["updatedOn"] = c.timer.Now()
		} else {
			doc["createdOn"] = c.timer.Now()
		}

		_, err = c.col.UpdateOne(
			ctx,
			bson.M{"_id": model.Id()},
			bson.M{"$set": doc},
			options.UpdateOne().SetUpsert(true),
		)
		return struct{}{}, err
	})
}

func (c *odmCollection[T]) FindOneByID(ctx context.Context, id string) <-chan async.Result[*T] {
	return c.FindOne(ctx, bson.M{"_id": id})
}

func (c *odmCollection[T]) FindOne(ctx context.Context, filters bson.M) <-chan async.Result[*T] {
	return async.Go(func() (*T, error) {
		if filters == nil {
			return nil, errors.New("filters cannot be nil for FindOne")
		}

		doc := c.col.FindOne(ctx, filters)
		if err := doc.Err(); err != nil {
			return nil, err
		}
		model := new(T)
		err := doc.Decode(model)
		return model, err
	})
}

func (c *odmCollection[T]) Find(ctx context.Context, filters bson.M, sort bson.D, limit, skip int64) <-chan async.Result[[]T] {
	return async.Go(func() ([]T, error) {
		if filters == nil {
			filters = bson.M{} // Default to empty filter if none provided
		}

		findOpts := options.Find().SetLimit(limit).SetSkip(skip)
		if sort != nil {
			findOpts.SetSort(sort)
		}
		cursor, err := c.col.Find(ctx, filters, findOpts)
		if err != nil {
			return nil, err
		}
		var result []T
		err = cursor.All(ctx, &result)
		return result, err
	})
}

func (c *odmCollection[T]) DeleteByID(ctx context.Context, id string) <-chan async.Result[struct{}] {
	return c.DeleteOne(ctx, bson.M{"_id": id})
}

func (c *odmCollection[T]) DeleteOne(ctx context.Context, filters bson.M) <-chan async.Result[struct{}] {
	return async.Go(func() (struct{}, error) {
		if filters == nil {
			return struct{}{}, errors.New("filters cannot be nil for DeleteOne")
		}

		_, err := c.col.DeleteOne(ctx, filters)
		return struct{}{}, err
	})
}

func (c *odmCollection[T]) Count(ctx context.Context, filters bson.M) <-chan async.Result[int64] {
	return async.Go(func() (int64, error) {
		return c.col.CountDocuments(ctx, filters)
	})
}

// Non-async method, since golang doesn't allow a separate type parameter for the function.
// Having out parameter and return async.Result can lead to confusion.
// This method is used to populate a slice with distinct values for a given field.
func (c *odmCollection[T]) DistinctInto(ctx context.Context, field string, filters bson.D, out any) error {
	if out == nil {
		return errors.New("output slice cannot be nil")
	}

	if filters == nil {
		filters = bson.D{} // Default to empty filter if none provided
	}

	res := c.col.Distinct(ctx, field, filters)

	if err := res.Err(); err != nil {
		return err
	}

	return res.Decode(out)
}

func (c *odmCollection[T]) Aggregate(ctx context.Context, pipeline mongo.Pipeline) <-chan async.Result[[]T] {
	return async.Go(func() ([]T, error) {
		cursor, err := c.col.Aggregate(ctx, pipeline)
		if err != nil {
			return nil, err
		}
		var result []T
		err = cursor.All(ctx, &result)
		return result, err
	})
}

func (c *odmCollection[T]) Exists(ctx context.Context, id string) <-chan async.Result[bool] {
	return async.Go(func() (bool, error) {
		count, err := c.col.CountDocuments(ctx, bson.M{"_id": id})
		if err != nil {
			return false, err
		}
		return count > 0, nil
	})
}

// VectorSearch performs a vector search using the specified embedding and options.
func (c *odmCollection[T]) VectorSearch(ctx context.Context, embedding []float32, params VectorSearchParams) <-chan async.Result[[]SearchHit[T]] {
	return async.Go(func() ([]SearchHit[T], error) {
		if len(embedding) == 0 || params.IndexName == "" || params.Path == "" || params.K <= 0 {
			return nil, errors.New("invalid input - embedding, index name, path, and K must be provided")
		}

		if params.Filter == nil {
			params.Filter = bson.M{} // Default to no filter if none provided
		}

		pipeline := mongo.Pipeline{
			bson.D{{
				Key: "$vectorSearch", Value: bson.D{
					{Key: "index", Value: params.IndexName},
					{Key: "path", Value: params.Path},
					{Key: "queryVector", Value: bson.NewVector(embedding).Binary()},
					{Key: "numCandidates", Value: params.NumCandidates},
					{Key: "limit", Value: params.K},
					{Key: "filter", Value: params.Filter},
				}}},
			bson.D{{
				Key: "$project", Value: bson.D{
					{Key: "score", Value: bson.D{{Key: "$meta", Value: "vectorSearchScore"}}},
					{Key: "doc", Value: "$$ROOT"},
				}}},
		}

		cursor, err := c.col.Aggregate(ctx, pipeline)
		if err != nil {
			return nil, err
		}

		var hits []SearchHit[T]
		if err = cursor.All(ctx, &hits); err != nil {
			return nil, err
		}
		return hits, nil
	})
}

func (c *odmCollection[T]) TermSearch(ctx context.Context, query string, params TermSearchParams) <-chan async.Result[[]SearchHit[T]] {
	return async.Go(func() ([]SearchHit[T], error) {
		if query == "" || params.IndexName == "" || params.Path == "" || params.Limit <= 0 {
			return nil, errors.New("invalid input - query, index name, path, and limit must be provided")
		}

		if params.Filter == nil {
			params.Filter = bson.M{}
		}

		pipeline := mongo.Pipeline{
			bson.D{{
				Key: "$search", Value: bson.D{
					{Key: "index", Value: params.IndexName},
					{Key: "text", Value: bson.D{
						{Key: "query", Value: query},
						{Key: "path", Value: params.Path},
					}},
				},
			}},
			bson.D{{Key: "$match", Value: params.Filter}},
			bson.D{{
				Key: "$project", Value: bson.D{
					{Key: "score", Value: bson.D{{Key: "$meta", Value: "searchScore"}}},
					{Key: "doc", Value: "$$ROOT"},
				},
			}},
			bson.D{{Key: "$limit", Value: params.Limit}},
		}

		cursor, err := c.col.Aggregate(ctx, pipeline)
		if err != nil {
			return nil, err
		}

		var hits []SearchHit[T]
		if err := cursor.All(ctx, &hits); err != nil {
			return nil, err
		}

		return hits, nil
	})
}

var convertToBson = func(model DbModel) (bson.M, error) {
	bsonBytes, err := bson.Marshal(model)
	if err != nil {
		return nil, err
	}
	var doc bson.M
	err = bson.Unmarshal(bsonBytes, &doc)
	return doc, err
}
