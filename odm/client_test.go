package odm

import (
	"context"
	"errors"
	"testing"

	"github.com/SaiNageswarS/go-api-boot/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

func TestNewMongoConn_PingFails(t *testing.T) {
	// simulate connection and ping error
	originalMongoConnect := mongoConnect
	mongoConnect = func(uri string) (MongoClient, error) {
		mockClient := new(MockMongoClient)
		mockClient.On("Ping", mock.Anything, mock.Anything).Return(errors.New("ping failed"))
		return mockClient, nil
	}

	defer func() {
		mongoConnect = originalMongoConnect
	}()

	testutil.WithEnv("MONGO_URI", "mongodb://test:27017", func(logger *testutil.MockLogger) {
		ProvideMongoClient()
		assert.True(t, logger.IsFatalCalled)
		assert.Equal(t, "Failed to ping MongoDB", logger.FatalMsg)
	})
}

func TestNewMongoConn_Success(t *testing.T) {
	originalMongoConnect := mongoConnect
	mongoConnect = func(uri string) (MongoClient, error) {
		mockClient := new(MockMongoClient)
		mockClient.On("Ping", mock.Anything, mock.Anything).Return(nil)
		return mockClient, nil
	}

	defer func() {
		mongoConnect = originalMongoConnect
	}()

	testutil.WithEnv("MONGO_URI", "mongodb://test:27017", func(logger *testutil.MockLogger) {
		client := ProvideMongoClient()
		assert.NotNil(t, client)
	})
}

func TestGetClient_EmptyURI(t *testing.T) {
	testutil.WithEnv("MONGO_URI", "", func(mLog *testutil.MockLogger) {
		ProvideMongoClient()
		assert.True(t, mLog.IsFatalCalled)
		assert.Equal(t, "MONGO_URI environment variable is not set", mLog.FatalMsg)
	})
}

func TestGetClient_Failure(t *testing.T) {
	originalMongoConnect := mongoConnect
	mongoConnect = func(uri string) (MongoClient, error) {
		return nil, errors.New("connect error")
	}

	defer func() {
		mongoConnect = originalMongoConnect
	}()

	testutil.WithEnv("MONGO_URI", "mongodb://test:27017", func(mLog *testutil.MockLogger) {
		ProvideMongoClient()
		assert.True(t, mLog.IsFatalCalled)
		assert.Equal(t, "Failed to connect to MongoDB", mLog.FatalMsg)
	})
}

type MockMongoClient struct {
	mock.Mock
}

func (m *MockMongoClient) Ping(ctx context.Context, rp *readpref.ReadPref) error {
	args := m.Called(ctx, rp)
	return args.Error(0)
}

func (m *MockMongoClient) Database(name string, opts ...options.Lister[options.DatabaseOptions]) *mongo.Database {
	args := m.Called(name, opts)
	return args.Get(0).(*mongo.Database)
}

func (m *MockMongoClient) Disconnect(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}
