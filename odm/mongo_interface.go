package odm

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

type CollectionInterface interface {
	UpdateOne(ctx context.Context, filter interface{}, update interface{}, opts ...options.Lister[options.UpdateOneOptions]) (*mongo.UpdateResult, error)
	FindOne(ctx context.Context, filter interface{}, opts ...options.Lister[options.FindOneOptions]) *mongo.SingleResult
	Find(ctx context.Context, filter interface{}, opts ...options.Lister[options.FindOptions]) (cur *mongo.Cursor, err error)
	DeleteOne(ctx context.Context, filter interface{}, opts ...options.Lister[options.DeleteOneOptions]) (*mongo.DeleteResult, error)
	Aggregate(ctx context.Context, pipeline interface{}, opts ...options.Lister[options.AggregateOptions]) (*mongo.Cursor, error)
	CountDocuments(ctx context.Context, filter interface{}, opts ...options.Lister[options.CountOptions]) (int64, error)
	Distinct(ctx context.Context, field string, filter any, opts ...options.Lister[options.DistinctOptions]) *mongo.DistinctResult
}

type MongoClient interface {
	Ping(context.Context, *readpref.ReadPref) error
	Database(name string, opts ...options.Lister[options.DatabaseOptions]) *mongo.Database
	Disconnect(ctx context.Context) error
}

type Timer interface {
	Now() int64
}

type DefaultTimer struct{}

func (d DefaultTimer) Now() int64 {
	return time.Now().Unix()
}
