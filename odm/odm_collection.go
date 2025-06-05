// odm/async.Result.go
package odm

import (
	"context"
	"time"

	"github.com/SaiNageswarS/go-api-boot/async"
	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

type OdmCollectionInterface[T DbModel] interface {
	Save(ctx context.Context, model T) <-chan async.Result[struct{}]
	FindOneByID(ctx context.Context, id string) <-chan async.Result[*T]
	FindOne(ctx context.Context, filters bson.M) <-chan async.Result[*T]
	Find(ctx context.Context, filters bson.M, sort bson.D, limit, skip int64) <-chan async.Result[[]T]
	DeleteByID(ctx context.Context, id string) <-chan async.Result[struct{}]
	DeleteOne(ctx context.Context, filters bson.M) <-chan async.Result[struct{}]
	Count(ctx context.Context, filters bson.M) <-chan async.Result[int64]
	Distinct(ctx context.Context, field string, filters bson.D, serverMaxTime time.Duration) <-chan async.Result[[]interface{}]
	Aggregate(ctx context.Context, pipeline mongo.Pipeline) <-chan async.Result[[]T]
	Exists(ctx context.Context, id string) <-chan async.Result[bool]
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
			options.Update().SetUpsert(true),
		)
		return struct{}{}, err
	})
}

func (c *odmCollection[T]) FindOneByID(ctx context.Context, id string) <-chan async.Result[*T] {
	return c.FindOne(ctx, bson.M{"_id": id})
}

func (c *odmCollection[T]) FindOne(ctx context.Context, filters bson.M) <-chan async.Result[*T] {
	return async.Go(func() (*T, error) {
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
		_, err := c.col.DeleteOne(ctx, filters)
		return struct{}{}, err
	})
}

func (c *odmCollection[T]) Count(ctx context.Context, filters bson.M) <-chan async.Result[int64] {
	return async.Go(func() (int64, error) {
		return c.col.CountDocuments(ctx, filters)
	})
}

func (c *odmCollection[T]) Distinct(ctx context.Context, field string, filters bson.D, serverMaxTime time.Duration) <-chan async.Result[[]interface{}] {
	return async.Go(func() ([]interface{}, error) {
		opts := options.Distinct().SetMaxTime(serverMaxTime)
		return c.col.Distinct(ctx, field, filters, opts)
	})
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
			logger.Error("Exists check failed", zap.Error(err))
			return false, err
		}
		return count > 0, nil
	})
}

func convertToBson(model DbModel) (bson.M, error) {
	bsonBytes, err := bson.Marshal(model)
	if err != nil {
		return nil, err
	}
	var doc bson.M
	err = bson.Unmarshal(bsonBytes, &doc)
	return doc, err
}
