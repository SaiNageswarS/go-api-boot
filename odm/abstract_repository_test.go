package odm

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type TestModel struct {
	Name     string `bson:"name"`
	PhotoUrl string `bson:"photoUrl"`
	Email    string `bson:"email"`
}

func (m *TestModel) Id() string {
	return "rg"
}

type TestRepository struct {
	UnimplementedBootRepository[TestModel]
}

func TestSave(t *testing.T) {
	collection := &MockCollection{}
	baseRepo := UnimplementedBootRepository[TestModel]{collection: collection, timer: &MockTimer{}}
	repo := &TestRepository{baseRepo}

	expectedFilter := bson.M{"_id": "rg"}
	expectedUpdate := bson.M{"$set": bson.M{"_id": "rg", "name": "Rick", "photoUrl": "rick.png", "email": "rick@gmail.com", "createdOn": int64(2024)}}

	collection.On("UpdateOne", mock.Anything, expectedFilter, expectedUpdate, mock.Anything).Return(&mongo.UpdateResult{}, nil)
	// return 0 to indicate that the document does not exist
	collection.On("CountDocuments", mock.Anything, bson.M{"_id": "rg"}, mock.Anything).Return(int64(0), nil)

	err := <-repo.Save(&TestModel{Name: "Rick", PhotoUrl: "rick.png", Email: "rick@gmail.com"})
	require.NoError(t, err)
	collection.AssertCalled(t, "UpdateOne", mock.Anything, expectedFilter, expectedUpdate, mock.Anything)
}

func TestSaveErr(t *testing.T) {
	collection := &MockCollection{}
	baseRepo := UnimplementedBootRepository[TestModel]{collection: collection, timer: &MockTimer{}}
	repo := &TestRepository{baseRepo}

	expectedErr := fmt.Errorf("failed to save")

	collection.On("UpdateOne", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mongo.UpdateResult{}, expectedErr)
	collection.On("CountDocuments", mock.Anything, mock.Anything, mock.Anything).Return(int64(0), nil)

	err := <-repo.Save(&TestModel{Name: "Rick", PhotoUrl: "rick.png", Email: "rick@gmail.com"})
	require.ErrorIs(t, err, expectedErr)
}

func TestUpdate(t *testing.T) {
	collection := &MockCollection{}
	baseRepo := UnimplementedBootRepository[TestModel]{collection: collection, timer: &MockTimer{}}
	repo := &TestRepository{baseRepo}

	expectedFilter := bson.M{"_id": "rg"}
	expectedUpdate := bson.M{"$set": bson.M{"_id": "rg", "name": "Rick", "photoUrl": "rick.png", "email": "rick@gmail.com", "updatedOn": int64(2024)}}

	collection.On("UpdateOne", mock.Anything, expectedFilter, expectedUpdate, mock.Anything).Return(&mongo.UpdateResult{}, nil)
	// return 1 to indicate that the document exists
	collection.On("CountDocuments", mock.Anything, bson.M{"_id": "rg"}, mock.Anything).Return(int64(1), nil)

	err := <-repo.Save(&TestModel{Name: "Rick", PhotoUrl: "rick.png", Email: "rick@gmail.com"})
	require.NoError(t, err)
	collection.AssertCalled(t, "UpdateOne", mock.Anything, expectedFilter, expectedUpdate, mock.Anything)
}

func TestFindOneById(t *testing.T) {
	collection := &MockCollection{}
	baseRepo := UnimplementedBootRepository[TestModel]{collection: collection, timer: &MockTimer{}}
	repo := &TestRepository{baseRepo}

	expectedFilter := bson.M{"_id": "rg"}
	expectedModel := &TestModel{Name: "Rick", PhotoUrl: "rick.png", Email: "rick@gmail.com"}

	findOneResult := mongo.NewSingleResultFromDocument(expectedModel, nil, nil)
	collection.On("FindOne", mock.Anything, expectedFilter, mock.Anything).Return(findOneResult)

	resChan, errChan := repo.FindOneById("rg")
	select {
	case res := <-resChan:
		require.Equal(t, expectedModel, res)
	case err := <-errChan:
		require.NoError(t, err)
	}
}

func TestAggregate(t *testing.T) {
	collection := &MockCollection{}
	baseRepo := UnimplementedBootRepository[TestModel]{collection: collection, timer: &MockTimer{}}
	repo := &TestRepository{baseRepo}

	expectedPipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{{Key: "Name", Value: "Rick"}}}},
		bson.D{{Key: "$sample", Value: bson.D{{Key: "size", Value: 1}}}},
	}

	expectedModels := []TestModel{
		{Name: "Rick", PhotoUrl: "rick.png", Email: "rick.ag@gmail.com"},
		{Name: "Rick", PhotoUrl: "rickPt.png", Email: "rick.pt@gmail.com"},
	}

	aggregateResult, _ := mongo.NewCursorFromDocuments(toInterface(expectedModels), nil, nil)
	collection.On("Aggregate", mock.Anything, expectedPipeline, mock.Anything).Return(aggregateResult, nil)

	resChan, errChan := repo.Aggregate(expectedPipeline)
	select {
	case res := <-resChan:
		require.Equal(t, expectedModels, res)
	case err := <-errChan:
		require.NoError(t, err)
	}
}

func TestDeleteById(t *testing.T) {
	collection := &MockCollection{}
	baseRepo := UnimplementedBootRepository[TestModel]{collection: collection, timer: &MockTimer{}}
	repo := &TestRepository{baseRepo}

	expectedFilter := bson.M{"_id": "rg"}

	collection.On("DeleteOne", mock.Anything, expectedFilter, mock.Anything).Return(&mongo.DeleteResult{DeletedCount: 1}, nil)

	err := <-repo.DeleteById("rg")
	require.NoError(t, err)
}

type MockCollection struct {
	mock.Mock
}

func (m *MockCollection) UpdateOne(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
	args := m.Called(ctx, filter, update, opts)
	return args.Get(0).(*mongo.UpdateResult), args.Error(1)
}

func (m *MockCollection) FindOne(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
	args := m.Called(ctx, filter, opts)
	return args.Get(0).(*mongo.SingleResult)
}

func (m *MockCollection) Find(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (cur *mongo.Cursor, err error) {
	args := m.Called(ctx, filter, opts)
	return args.Get(0).(*mongo.Cursor), args.Error(1)
}

func (m *MockCollection) DeleteOne(ctx context.Context, filter interface{}, opts ...*options.DeleteOptions) (*mongo.DeleteResult, error) {
	args := m.Called(ctx, filter, opts)
	return args.Get(0).(*mongo.DeleteResult), args.Error(1)
}

func (m *MockCollection) Aggregate(ctx context.Context, pipeline interface{}, opts ...*options.AggregateOptions) (*mongo.Cursor, error) {
	args := m.Called(ctx, pipeline, opts)
	return args.Get(0).(*mongo.Cursor), args.Error(1)
}

func (m *MockCollection) CountDocuments(ctx context.Context, filter interface{}, opts ...*options.CountOptions) (int64, error) {
	args := m.Called(ctx, filter, opts)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockCollection) Distinct(ctx context.Context, fieldName string, filter interface{}, opts ...*options.DistinctOptions) ([]interface{}, error) {
	args := m.Called(ctx, fieldName, filter, opts)
	return args.Get(0).([]interface{}), args.Error(1)
}

type MockTimer struct{}

func (m *MockTimer) Now() int64 {
	return 2024
}

func toInterface(models []TestModel) []interface{} {
	interfaces := make([]interface{}, len(models))
	for i, model := range models {
		interfaces[i] = model
	}
	return interfaces
}
