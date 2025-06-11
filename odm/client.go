package odm

import (
	"context"
	"errors"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var mongoConnect = func(uri string) (MongoClient, error) {
	opts := options.Client().ApplyURI(uri)
	return mongo.Connect(opts)
}

func GetClient() (MongoClient, error) {
	mongoUri := os.Getenv("MONGO_URI")
	if mongoUri == "" {
		return nil, errors.New("empty MongoDB URI")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongoConnect(mongoUri)
	if err != nil {
		return nil, err
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}

	return client, nil
}
