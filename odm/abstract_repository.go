package odm

import (
	"context"
	"time"

	"github.com/SaiNageswarS/go-api-boot/logger"
	"github.com/jinzhu/copier"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

type BootRepository[T any] interface {
	Save(model DbModel) chan error
	FindOneById(id string) (chan *T, chan error)
	IsExistsById(id string) bool
	CountDocuments(filters bson.M) (chan int64, chan error)
	Distinct(fieldName string, filters bson.D, serverMaxTime time.Duration) (chan []interface{}, chan error)
	FindOne(filters bson.M) (chan *T, chan error)
	Find(filters bson.M, sort bson.D, limit, skip int64) (chan []T, chan error)
	DeleteById(id string) chan error
	DeleteOne(filters bson.M) chan error
	GetModel(proto interface{}) *T
	Aggregate(pipeline mongo.Pipeline) (chan []T, chan error)
}

type UnimplementedBootRepository[T any] struct {
	collection CollectionInterface
	timer      Timer
}

func NewUnimplementedBootRepository[T any](options ...Option) UnimplementedBootRepository[T] {
	config := NewConfig(options...)

	if config.Client == nil {
		config.Client = GetClient()
	}

	return UnimplementedBootRepository[T]{
		collection: config.Client.Database(config.Database).Collection(config.CollectionName),
		timer:      DefaultTimer{},
	}
}

func (r *UnimplementedBootRepository[T]) Save(model DbModel) chan error {
	ch := make(chan error)

	go func() {
		id := model.Id()
		document, err := convertToBson(model)
		if err != nil {
			ch <- err
			return
		}

		document["_id"] = id
		if r.IsExistsById(id) {
			document["updatedOn"] = r.timer.Now()
		} else {
			document["createdOn"] = r.timer.Now()
		}

		_, err = r.collection.UpdateOne(
			context.Background(),
			bson.M{"_id": id},
			bson.M{"$set": document},
			options.Update().SetUpsert(true))

		if err != nil {
			ch <- err
			return
		}

		ch <- nil
	}()

	return ch
}

// Finds one object based on Id.
func (r *UnimplementedBootRepository[T]) FindOneById(id string) (chan *T, chan error) {
	return r.FindOne(bson.M{"_id": id})
}

// checks if a record exists by id.
// Synchronous becuase it is expected to be very light-weighted without deserialization etc.
func (r *UnimplementedBootRepository[T]) IsExistsById(id string) bool {
	count, err := r.collection.CountDocuments(context.Background(), bson.M{"_id": id})
	if err != nil {
		logger.Error("Failed getting count of object", zap.Error(err))
		return false
	}

	return count > 0
}

// Finds documents count on filters.
func (r *UnimplementedBootRepository[T]) CountDocuments(filters bson.M) (chan int64, chan error) {
	resultChan := make(chan int64)
	errorChan := make(chan error)

	go func() {
		count, err := r.collection.CountDocuments(context.Background(), filters)

		if err != nil {
			errorChan <- err
		} else {
			resultChan <- count
		}
	}()

	return resultChan, errorChan
}

// Finds all unique values for a field
func (r *UnimplementedBootRepository[T]) Distinct(fieldName string, filters bson.D, serverMaxTime time.Duration) (chan []interface{}, chan error) {
	resultChan := make(chan []interface{})
	errorChan := make(chan error)

	go func() {
		opts := &options.DistinctOptions{}
		opts.SetMaxTime(serverMaxTime)
		distinctValues, err := r.collection.Distinct(context.Background(), fieldName, filters, opts)

		if err != nil {
			errorChan <- err
		} else {
			resultChan <- distinctValues
		}
	}()

	return resultChan, errorChan
}

// Finds one object based on filters.
func (r *UnimplementedBootRepository[T]) FindOne(filters bson.M) (chan *T, chan error) {
	resultChan := make(chan *T)
	errorChan := make(chan error)

	go func() {
		document := r.collection.FindOne(context.Background(), filters)

		if document.Err() != nil {
			errorChan <- document.Err()
		} else {
			model := new(T)
			document.Decode(model)
			resultChan <- model
		}
	}()

	return resultChan, errorChan
}

func (r *UnimplementedBootRepository[T]) Find(filters bson.M, sort bson.D, limit, skip int64) (chan []T, chan error) {
	resultChan := make(chan []T)
	errorChan := make(chan error)

	go func() {
		findOptions := options.Find()
		if sort != nil {
			findOptions.SetSort(sort)
		}
		findOptions.SetLimit(limit)
		findOptions.SetSkip(skip)

		cursor, err := r.collection.Find(context.Background(), filters, findOptions)
		if err != nil {
			errorChan <- err
			return
		}

		var result []T
		if err = cursor.All(context.Background(), &result); err != nil {
			errorChan <- err
			return
		}

		resultChan <- result
	}()

	return resultChan, errorChan
}

func (r *UnimplementedBootRepository[T]) DeleteById(id string) chan error {
	ch := make(chan error)

	go func() {
		_, err := r.collection.DeleteOne(context.Background(), bson.M{"_id": id})
		ch <- err
	}()

	return ch
}

func (r *UnimplementedBootRepository[T]) DeleteOne(filters bson.M) chan error {
	ch := make(chan error)

	go func() {
		_, err := r.collection.DeleteOne(context.Background(), filters)
		ch <- err
	}()

	return ch
}

// Gets an instance of model from proto or othe object.
func (r *UnimplementedBootRepository[T]) GetModel(proto interface{}) *T {
	model := new(T)
	copier.Copy(model, proto)
	return model
}

func (r *UnimplementedBootRepository[T]) Aggregate(pipeline mongo.Pipeline) (chan []T, chan error) {
	resultChan := make(chan []T)
	errorChan := make(chan error)

	go func() {
		cursor, err := r.collection.Aggregate(context.Background(), pipeline)
		if err != nil {
			errorChan <- err
			return
		}
		defer cursor.Close(context.Background())
		var result []T
		if err = cursor.All(context.Background(), &result); err != nil {
			errorChan <- err
			return
		}
		resultChan <- result
	}()

	return resultChan, errorChan
}

func convertToBson(model DbModel) (bson.M, error) {
	pByte, err := bson.Marshal(model)
	if err != nil {
		return nil, err
	}

	var update bson.M
	err = bson.Unmarshal(pByte, &update)
	if err != nil {
		return nil, err
	}
	return update, nil
}
