package odm

import (
	"context"
	"os"
	"time"

	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

var mongoConnect = func(uri string) (MongoClient, error) {
	opts := options.Client().ApplyURI(uri)
	return mongo.Connect(opts)
}

func ProvideMongoClient() MongoClient {
	mongoUri := os.Getenv("MONGO_URI")
	if mongoUri == "" {
		// Providers are designed for dependency injection.
		// If the MONGO_URI is not set, we log a fatal error.
		logger.Fatal("MONGO_URI environment variable is not set")
		return nil // This will never be reached, but it's good practice to return nil here.
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongoConnect(mongoUri)
	if err != nil {
		logger.Fatal("Failed to connect to MongoDB", zap.Error(err))
		return nil
	}

	if err := client.Ping(ctx, nil); err != nil {
		logger.Fatal("Failed to ping MongoDB", zap.Error(err))
		return nil
	}

	return client
}
