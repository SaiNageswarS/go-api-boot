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

type AbstractRepository[T any] struct {
	Database       string
	CollectionName string
}

func (r *AbstractRepository[T]) db() *mongo.Database {
	return GetClient().Database(r.Database)
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

func (r *AbstractRepository[T]) Save(model DbModel) chan error {
	ch := make(chan error)

	go func() {
		id := model.Id()
		collection := r.db().Collection(r.CollectionName)
		document, err := convertToBson(model)
		if err != nil {
			ch <- err
			return
		}

		document["_id"] = id
		if r.IsExistsById(id) {
			document["updatedOn"] = time.Now().Unix()
		} else {
			document["createdOn"] = time.Now().Unix()
		}

		_, err = collection.UpdateOne(
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
func (r *AbstractRepository[T]) FindOneById(id string) (chan *T, chan error) {
	return r.FindOne(bson.M{"_id": id})
}

// checks if a record exists by id.
// Synchronous becuase it is expected to be very light-weighted without deserialization etc.
func (r *AbstractRepository[T]) IsExistsById(id string) bool {
	collection := r.db().Collection(r.CollectionName)
	count, err := collection.CountDocuments(context.Background(), bson.M{"_id": id})
	if err != nil {
		logger.Error("Failed getting count of object", zap.Error(err))
		return false
	}

	return count > 0
}

// Finds one object based on filters.
func (r *AbstractRepository[T]) FindOne(filters bson.M) (chan *T, chan error) {
	resultChan := make(chan *T)
	errorChan := make(chan error)

	go func() {
		collection := r.db().Collection(r.CollectionName)
		document := collection.FindOne(context.Background(), filters)

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

func (r *AbstractRepository[T]) Find(filters bson.M, sort bson.D, limit, skip int64) (chan []T, chan error) {
	resultChan := make(chan []T)
	errorChan := make(chan error)

	go func() {
		collection := r.db().Collection(r.CollectionName)

		findOptions := options.Find()
		if sort != nil {
			findOptions.SetSort(sort)
		}
		findOptions.SetLimit(limit)
		findOptions.SetSkip(skip)

		cursor, err := collection.Find(context.Background(), filters, findOptions)
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

func (r *AbstractRepository[T]) DeleteById(id string) chan error {
	ch := make(chan error)

	go func() {
		collection := r.db().Collection(r.CollectionName)
		_, err := collection.DeleteOne(context.Background(), bson.M{"_id": id})
		ch <- err
	}()

	return ch
}

func (r *AbstractRepository[T]) DeleteOne(filters bson.M) chan error {
	ch := make(chan error)

	go func() {
		collection := r.db().Collection(r.CollectionName)
		_, err := collection.DeleteOne(context.Background(), filters)
		ch <- err
	}()

	return ch
}

// Gets an instance of model from proto or othe object.
func (r *AbstractRepository[T]) GetModel(proto interface{}) *T {
	model := new(T)
	copier.Copy(model, proto)
	return model
}
