// odm/result.go
package odm

import (
	"context"
	"time"

	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

type Result[T any] struct {
	Data T
	Err  error
}

func Await[T any](ch <-chan Result[T]) (T, error) {
	res := <-ch
	return res.Data, res.Err
}

type OdmCollectionInterface[T DbModel] interface {
	Save(ctx context.Context, model T) <-chan Result[struct{}]
	FindOneByID(ctx context.Context, id string) <-chan Result[*T]
	FindOne(ctx context.Context, filters bson.M) <-chan Result[*T]
	Find(ctx context.Context, filters bson.M, sort bson.D, limit, skip int64) <-chan Result[[]T]
	DeleteByID(ctx context.Context, id string) <-chan Result[struct{}]
	DeleteOne(ctx context.Context, filters bson.M) <-chan Result[struct{}]
	Count(ctx context.Context, filters bson.M) <-chan Result[int64]
	Distinct(ctx context.Context, field string, filters bson.D, serverMaxTime time.Duration) <-chan Result[[]interface{}]
	Aggregate(ctx context.Context, pipeline mongo.Pipeline) <-chan Result[[]T]
	Exists(ctx context.Context, id string) <-chan Result[bool]
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

// To avoid copying of model for save, use Save as below
// lead := &db.LeadModel { Name: "Lead1" }
// _, err := odm.Await(odm.CollectionOf[*db.LeadModel](s.mongo, tenant).Save(ctx, lead))
func (c *odmCollection[T]) Save(ctx context.Context, model T) <-chan Result[struct{}] {
	out := make(chan Result[struct{}], 1)
	go func() {
		defer close(out)
		doc, err := convertToBson(model)
		if err != nil {
			out <- Result[struct{}]{Err: err}
			return
		}

		doc["_id"] = model.Id()
		exists, _ := Await(c.Exists(ctx, model.Id()))
		if exists {
			doc["updatedOn"] = c.timer.Now()
		} else {
			doc["createdOn"] = c.timer.Now()
		}

		_, err = c.col.UpdateOne(ctx, bson.M{"_id": model.Id()}, bson.M{"$set": doc}, options.Update().SetUpsert(true))
		out <- Result[struct{}]{Err: err}
	}()
	return out
}

func (c *odmCollection[T]) FindOneByID(ctx context.Context, id string) <-chan Result[*T] {
	return c.FindOne(ctx, bson.M{"_id": id})
}

func (c *odmCollection[T]) FindOne(ctx context.Context, filters bson.M) <-chan Result[*T] {
	out := make(chan Result[*T], 1)
	go func() {
		defer close(out)
		doc := c.col.FindOne(ctx, filters)
		if doc.Err() != nil {
			out <- Result[*T]{Err: doc.Err()}
			return
		}
		model := new(T)
		err := doc.Decode(model)
		out <- Result[*T]{Data: model, Err: err}
	}()
	return out
}

func (c *odmCollection[T]) Find(ctx context.Context, filters bson.M, sort bson.D, limit, skip int64) <-chan Result[[]T] {
	out := make(chan Result[[]T], 1)
	go func() {
		defer close(out)
		findOpts := options.Find().SetLimit(limit).SetSkip(skip)
		if sort != nil {
			findOpts.SetSort(sort)
		}
		cursor, err := c.col.Find(ctx, filters, findOpts)
		if err != nil {
			out <- Result[[]T]{Err: err}
			return
		}
		var result []T
		err = cursor.All(ctx, &result)
		out <- Result[[]T]{Data: result, Err: err}
	}()
	return out
}

func (c *odmCollection[T]) DeleteByID(ctx context.Context, id string) <-chan Result[struct{}] {
	return c.DeleteOne(ctx, bson.M{"_id": id})
}

func (c *odmCollection[T]) DeleteOne(ctx context.Context, filters bson.M) <-chan Result[struct{}] {
	out := make(chan Result[struct{}], 1)
	go func() {
		defer close(out)
		_, err := c.col.DeleteOne(ctx, filters)
		out <- Result[struct{}]{Err: err}
	}()
	return out
}

func (c *odmCollection[T]) Count(ctx context.Context, filters bson.M) <-chan Result[int64] {
	out := make(chan Result[int64], 1)
	go func() {
		defer close(out)
		count, err := c.col.CountDocuments(ctx, filters)
		out <- Result[int64]{Data: count, Err: err}
	}()
	return out
}

func (c *odmCollection[T]) Distinct(ctx context.Context, field string, filters bson.D, serverMaxTime time.Duration) <-chan Result[[]interface{}] {
	out := make(chan Result[[]interface{}], 1)
	go func() {
		defer close(out)
		opts := &options.DistinctOptions{}
		opts.SetMaxTime(serverMaxTime)
		res, err := c.col.Distinct(ctx, field, filters, opts)
		out <- Result[[]interface{}]{Data: res, Err: err}
	}()
	return out
}

func (c *odmCollection[T]) Aggregate(ctx context.Context, pipeline mongo.Pipeline) <-chan Result[[]T] {
	out := make(chan Result[[]T], 1)
	go func() {
		defer close(out)
		cursor, err := c.col.Aggregate(ctx, pipeline)
		if err != nil {
			out <- Result[[]T]{Err: err}
			return
		}
		var result []T
		err = cursor.All(ctx, &result)
		out <- Result[[]T]{Data: result, Err: err}
	}()
	return out
}

func (c *odmCollection[T]) Exists(ctx context.Context, id string) <-chan Result[bool] {
	out := make(chan Result[bool], 1)
	go func() {
		defer close(out)
		count, err := c.col.CountDocuments(ctx, bson.M{"_id": id})
		if err != nil {
			logger.Error("Exists check failed", zap.Error(err))
			out <- Result[bool]{Data: false, Err: err}
			return
		}
		out <- Result[bool]{Data: count > 0, Err: nil}
	}()
	return out
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
