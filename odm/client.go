package odm

import (
	"context"
	"crypto/tls"
	"errors"
	"time"

	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

var mongoConnect = func(ctx context.Context, uri string) (MongoClient, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	opts := options.Client().ApplyURI(uri).SetTLSConfig(tlsConfig)

	return mongo.Connect(ctx, opts)
}

// GetClient returns a singleton Mongo client, initialized once.
func GetClient(mongoUri string) (MongoClient, error) {
	if mongoUri == "" {
		return nil, errors.New("empty MongoDB URI")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongoConnect(ctx, mongoUri)
	if err != nil {
		logger.Error("Mongo connection failed", zap.Error(err))
		return nil, err
	}

	if err := client.Ping(ctx, nil); err != nil {
		logger.Error("Mongo ping failed", zap.Error(err))
		return nil, err
	}

	return client, nil
}
