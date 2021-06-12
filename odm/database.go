package odm

import (
	"context"
	"crypto/tls"
	"os"
	"time"

	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

var database *mongo.Database = nil

func newMongoDb() *mongo.Database {
	mongoUri := os.Getenv("MONGO_URI")
	databaseName := os.Getenv("DATABASE")

	mongoOpts := options.Client().ApplyURI(mongoUri)
	mongoOpts.TLSConfig.MinVersion = tls.VersionTLS12
	mongoOpts.TLSConfig.InsecureSkipVerify = true

	client, err := mongo.NewClient(mongoOpts)
	if err != nil {
		logger.Fatal("Failed to connect to mongo", zap.Error(err))
	}

	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	err = client.Connect(ctx)
	if err != nil {
		logger.Fatal("Failed to connect to mongo", zap.Error(err))
	}

	err = client.Ping(ctx, nil)
	if err != nil {
		logger.Fatal("Failed to connect to mongo", zap.Error(err))
	}

	db := client.Database(databaseName)
	return db
}

func GetDatabase() *mongo.Database {
	if database != nil {
		return database
	}

	database = newMongoDb()
	return database
}
