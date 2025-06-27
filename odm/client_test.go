package odm

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/SaiNageswarS/go-api-boot/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.uber.org/zap"
)

func TestNewMongoConn_PingFails(t *testing.T) {
	// simulate connection and ping error
	originalMongoConnect := mongoConnect
	mongoConnect = func(uri string) (MongoClient, error) {
		mockClient := new(MockMongoClient)
		mockClient.On("Ping", mock.Anything, mock.Anything).Return(errors.New("ping failed"))
		return mockClient, nil
	}

	originalMongoUri := os.Getenv("MONGO_URI")
	os.Setenv("MONGO_URI", "mongodb://test:27017")
	defer func() {
		mongoConnect = originalMongoConnect
		os.Setenv("MONGO_URI", originalMongoUri)
	}()

	// Replace the logger's Fatal function with a mock
	mLog := MockLogger{}
	originalFatal := logger.Fatal
	defer func() {
		logger.Fatal = originalFatal
	}()
	logger.Fatal = mLog.Fatal

	ProvideMongoClient()
	assert.True(t, mLog.isFatalCalled)
	assert.Equal(t, "Failed to ping MongoDB", mLog.fatalMsg)
}

func TestNewMongoConn_Success(t *testing.T) {
	originalMongoConnect := mongoConnect
	mongoConnect = func(uri string) (MongoClient, error) {
		mockClient := new(MockMongoClient)
		mockClient.On("Ping", mock.Anything, mock.Anything).Return(nil)
		return mockClient, nil
	}

	originalMongoUri := os.Getenv("MONGO_URI")
	os.Setenv("MONGO_URI", "mongodb://test:27017")
	defer func() {
		mongoConnect = originalMongoConnect
		os.Setenv("MONGO_URI", originalMongoUri)
	}()

	client := ProvideMongoClient()
	assert.NotNil(t, client)
}

func TestGetClient_EmptyURI(t *testing.T) {
	originalMongoUri := os.Getenv("MONGO_URI")
	os.Unsetenv("MONGO_URI")
	defer func() {
		os.Setenv("MONGO_URI", originalMongoUri)
	}()

	mLog := MockLogger{}
	originalFatal := logger.Fatal
	defer func() {
		logger.Fatal = originalFatal
	}()
	logger.Fatal = mLog.Fatal

	ProvideMongoClient()
	assert.True(t, mLog.isFatalCalled)
	assert.Equal(t, "MONGO_URI environment variable is not set", mLog.fatalMsg)
}

func TestGetClient_Failure(t *testing.T) {
	originalMongoConnect := mongoConnect
	mongoConnect = func(uri string) (MongoClient, error) {
		return nil, errors.New("connect error")
	}

	originalMongoUri := os.Getenv("MONGO_URI")
	os.Setenv("MONGO_URI", "mongodb://test:27017")
	defer func() {
		mongoConnect = originalMongoConnect
		os.Setenv("MONGO_URI", originalMongoUri)
	}()

	mLog := MockLogger{}
	originalFatal := logger.Fatal
	defer func() {
		logger.Fatal = originalFatal
	}()
	logger.Fatal = mLog.Fatal

	ProvideMongoClient()
	assert.True(t, mLog.isFatalCalled)
	assert.Equal(t, "Failed to connect to MongoDB", mLog.fatalMsg)
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

type MockLogger struct {
	isFatalCalled bool
	fatalMsg      string
}

func (m MockLogger) Fatal(msg string, fields ...zap.Field) {
	m.isFatalCalled = true
	m.fatalMsg = msg
}
