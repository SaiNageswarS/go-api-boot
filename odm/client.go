package odm

import (
	"context"
	"crypto/tls"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

var (
	connection MongoClient
	once       sync.Once
	connErr    error
)

var mongoConnect = func(ctx context.Context, uri string) (MongoClient, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	opts := options.Client().ApplyURI(uri).SetTLSConfig(tlsConfig)

	return mongo.Connect(ctx, opts)
}

// newMongoConn creates and returns a new mongo client connection
func newMongoConn(ctx context.Context) (MongoClient, error) {
	mongoUri := os.Getenv("MONGO-URI")
	if mongoUri == "" {
		return nil, errors.New("MONGO-URI environment variable is not set")
	}

	client, err := mongoConnect(ctx, mongoUri)
	if err != nil {
		return nil, err
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}
	return client, nil
}

// GetClient returns a singleton Mongo client, initialized once.
func GetClient() (MongoClient, error) {
	once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client, err := newMongoConn(ctx)
		if err != nil {
			connErr = err
			logger.Error("Mongo connection failed", zap.Error(err))
			return
		}

		connection = client
	})

	return connection, connErr
}
