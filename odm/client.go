package odm

import (
	"context"
	"crypto/tls"
	"errors"
	"sync"
	"time"

	"github.com/SaiNageswarS/go-api-boot/config"
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

// GetClient returns a singleton Mongo client, initialized once.
func GetClient(config *config.BootConfig) (MongoClient, error) {
	if config.MongoUri == "" {
		return nil, errors.New("MongoUri config is not set")
	}

	once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client, err := mongoConnect(ctx, config.MongoUri)
		if err != nil {
			connErr = err
			logger.Error("Mongo connection failed", zap.Error(err))
			return
		}

		if err := client.Ping(ctx, nil); err != nil {
			connErr = err
			logger.Error("Mongo ping failed", zap.Error(err))
			return
		}

		connection = client
	})

	return connection, connErr
}
