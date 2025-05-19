package odm

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/SaiNageswarS/go-api-boot/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

func TestNewMongoConn_ErrorWhenEnvMissing(t *testing.T) {
	resetMongoClientGlobals()
	config := &config.BaseConfig{}

	client, err := newMongoConn(context.Background(), config)
	assert.Nil(t, client)
	assert.EqualError(t, err, "MONGO-URI environment variable is not set")
}

func TestNewMongoConn_PingFails(t *testing.T) {
	resetMongoClientGlobals()

	config := &config.BaseConfig{MongoUri: "mongodb://test:27017"}

	// simulate connection and ping error
	originalMongoConnect := mongoConnect
	mongoConnect = func(ctx context.Context, uri string) (MongoClient, error) {
		mockClient := new(MockMongoClient)
		mockClient.On("Ping", ctx, mock.Anything).Return(errors.New("ping failed"))
		return mockClient, nil
	}
	defer func() { mongoConnect = originalMongoConnect }()

	client, err := GetClient(config)
	assert.Nil(t, client)
	assert.EqualError(t, err, "ping failed")
}

func TestNewMongoConn_Success(t *testing.T) {
	resetMongoClientGlobals()

	config := &config.BaseConfig{MongoUri: "mongodb://test:27017"}

	originalMongoConnect := mongoConnect
	mongoConnect = func(ctx context.Context, uri string) (MongoClient, error) {
		mockClient := new(MockMongoClient)
		mockClient.On("Ping", ctx, mock.Anything).Return(nil)
		return mockClient, nil
	}
	defer func() { mongoConnect = originalMongoConnect }()

	client, err := GetClient(config)
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

func TestGetClient_Success(t *testing.T) {
	resetMongoClientGlobals()

	config := &config.BaseConfig{MongoUri: "mongodb://test:27017"}

	originalMongoConnect := mongoConnect
	mongoConnect = func(ctx context.Context, uri string) (MongoClient, error) {
		mockMongoClient := new(MockMongoClient)
		mockMongoClient.On("Ping", ctx, mock.Anything).Return(nil)
		return mockMongoClient, nil
	}
	defer func() { mongoConnect = originalMongoConnect }()

	client, err := GetClient(config)
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

func TestGetClient_Failure(t *testing.T) {
	resetMongoClientGlobals()

	config := &config.BaseConfig{MongoUri: "mongodb://test:27017"}

	originalMongoConnect := mongoConnect
	mongoConnect = func(ctx context.Context, uri string) (MongoClient, error) {
		return nil, errors.New("connect error")
	}
	defer func() { mongoConnect = originalMongoConnect }()

	client, err := GetClient(config)
	assert.Nil(t, client)
	assert.EqualError(t, err, "connect error")
}

func resetMongoClientGlobals() {
	connection = nil
	connErr = nil
	once = sync.Once{}
}

type MockMongoClient struct {
	mock.Mock
}

func (m *MockMongoClient) Ping(ctx context.Context, rp *readpref.ReadPref) error {
	args := m.Called(ctx, rp)
	return args.Error(0)
}

func (m *MockMongoClient) Database(name string, opts ...*options.DatabaseOptions) *mongo.Database {
	args := m.Called(name, opts)
	return args.Get(0).(*mongo.Database)
}
