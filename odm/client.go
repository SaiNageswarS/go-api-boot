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
	connection *mongo.Client
	once       sync.Once
	connErr    error
)

// newMongoConn creates and returns a new mongo client connection
func newMongoConn(ctx context.Context) (*mongo.Client, error) {
	mongoUri := os.Getenv("MONGO-URI")
	if mongoUri == "" {
		return nil, errors.New("MONGO-URI environment variable is not set")
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	mongoOpts := options.Client().ApplyURI(mongoUri).SetTLSConfig(tlsConfig)

	client, err := mongo.Connect(ctx, mongoOpts)
	if err != nil {
		return nil, err
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}

	return client, nil
}

// GetClient returns a singleton Mongo client, initialized once.
func GetClient() (*mongo.Client, error) {
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
