package odm

import (
	"context"
	"reflect"

	"github.com/jinzhu/copier"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type AbstractRepository struct {
	CollectionName string
	Model          reflect.Type
}

type Result struct {
	Value interface{}
	Err   error
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

func (r *AbstractRepository) Save(model DbModel) chan error {
	ch := make(chan error)

	go func() {
		id := model.Id()
		collection := GetDatabase().Collection(r.CollectionName)
		document, err := convertToBson(model)
		if err != nil {
			ch <- err
			return
		}

		document["_id"] = id

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
func (r *AbstractRepository) FindOneById(id string) chan Result {
	ch := make(chan Result)

	go func() {
		res := <-r.FindOne(bson.M{"_id": id})
		ch <- res
	}()
	return ch
}

// Finds one object based on filters.
func (r *AbstractRepository) FindOne(filters bson.M) chan Result {
	ch := make(chan Result)

	go func() {
		collection := GetDatabase().Collection(r.CollectionName)
		document := collection.FindOne(context.Background(), filters)

		if document.Err() != nil {
			ch <- Result{Err: document.Err()}
			return
		}
		model := reflect.New(r.Model).Interface()
		document.Decode(model)
		ch <- Result{Value: model}
	}()
	return ch
}

func (r *AbstractRepository) Find(filters bson.M, sort bson.D, limit, skip int64) chan Result {
	ch := make(chan Result)

	go func() {
		collection := GetDatabase().Collection(r.CollectionName)

		findOptions := options.Find()
		if sort != nil {
			findOptions.SetSort(sort)
		}
		findOptions.SetLimit(limit)
		findOptions.SetSkip(skip)

		cursor, err := collection.Find(context.Background(), filters, findOptions)
		if err != nil {
			ch <- Result{Err: err}
			return
		}

		models := reflect.MakeSlice(reflect.SliceOf(r.Model), int(limit), int(limit)).Interface()
		if err = cursor.All(context.Background(), models); err != nil {
			ch <- Result{Err: err}
			return
		}
		ch <- Result{Value: models}
	}()
	return ch
}

// Gets an instance of model from proto or othe object.
func (r *AbstractRepository) GetModel(proto interface{}) interface{} {
	model := reflect.New(r.Model).Interface()
	copier.Copy(model, proto)
	return model
}
