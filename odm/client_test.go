package odm

import (
	"context"
	"errors"
	"os"
	"testing"

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
	os.Setenv("MONGO_URI", "mongodb://test:27017")
	defer func() {
		mongoConnect = originalMongoConnect
		os.Unsetenv("MONGO_URI")
	}()

	client, err := GetClient()
	assert.Nil(t, client)
	assert.EqualError(t, err, "ping failed")
}

func TestNewMongoConn_Success(t *testing.T) {
	originalMongoConnect := mongoConnect
	mongoConnect = func(uri string) (MongoClient, error) {
		mockClient := new(MockMongoClient)
		mockClient.On("Ping", mock.Anything, mock.Anything).Return(nil)
		return mockClient, nil
	}
	os.Setenv("MONGO_URI", "mongodb://test:27017")
	defer func() {
		mongoConnect = originalMongoConnect
		os.Unsetenv("MONGO_URI")
	}()

	client, err := GetClient()
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

func TestGetClient_Success(t *testing.T) {
	originalMongoConnect := mongoConnect
	mongoConnect = func(uri string) (MongoClient, error) {
		mockMongoClient := new(MockMongoClient)
		mockMongoClient.On("Ping", mock.Anything, mock.Anything).Return(nil)
		return mockMongoClient, nil
	}
	os.Setenv("MONGO_URI", "mongodb://test:27017")
	defer func() {
		mongoConnect = originalMongoConnect
		os.Unsetenv("MONGO_URI")
	}()

	client, err := GetClient()
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

func TestGetClient_EmptyURI(t *testing.T) {
	client, err := GetClient()
	assert.Nil(t, client)
	assert.EqualError(t, err, "empty MongoDB URI")
}

func TestGetClient_Failure(t *testing.T) {
	originalMongoConnect := mongoConnect
	mongoConnect = func(uri string) (MongoClient, error) {
		return nil, errors.New("connect error")
	}
	os.Setenv("MONGO_URI", "mongodb://test:27017")
	defer func() {
		mongoConnect = originalMongoConnect
		os.Unsetenv("MONGO_URI")
	}()

	client, err := GetClient()
	assert.Nil(t, client)
	assert.EqualError(t, err, "connect error")
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
