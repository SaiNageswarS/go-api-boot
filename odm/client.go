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

var connection *mongo.Client = nil

func newMongoConn() *mongo.Client {
	mongoUri := os.Getenv("MONGO_URI")

	mongoOpts := options.Client().ApplyURI(mongoUri)
	mongoOpts.TLSConfig.MinVersion = tls.VersionTLS12
	// make sure to install ca-certs in docker image.
	// mongoOpts.TLSConfig.InsecureSkipVerify = true

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

	return client
}

func GetClient() *mongo.Client {
	if connection != nil {
		return connection
	}

	connection = newMongoConn()
	return connection
}
