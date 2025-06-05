package odm

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/SaiNageswarS/go-api-boot/async"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

/* ─────────────────────────────
   Test scaffolding & fixtures
   ───────────────────────────── */

type testModel struct {
	Name     string `bson:"name"`
	PhotoUrl string `bson:"photoUrl"`
	Email    string `bson:"email"`
}

func (m testModel) Id() string             { return "rg" }
func (m testModel) CollectionName() string { return "test" }

type testOdmCollection struct {
	odmCollection[testModel]
}

type MockTimer struct{}

func (m *MockTimer) Now() int64 { return 2024 }

/* ─────────────────────────────
   Tests
   ───────────────────────────── */

func TestSave_Insert(t *testing.T) {
	ctx := context.Background()

	collection := &MockCollection{}
	baseRepo := odmCollection[testModel]{col: collection, timer: &MockTimer{}}
	repo := &testOdmCollection{baseRepo}

	expectedFilter := bson.M{"_id": "rg"}
	expectedUpdate := bson.M{
		"$set": bson.M{
			"_id":       "rg",
			"name":      "Rick",
			"photoUrl":  "rick.png",
			"email":     "rick@gmail.com",
			"createdOn": int64(2024),
		},
	}

	collection.
		On("UpdateOne", mock.Anything, expectedFilter, expectedUpdate, mock.Anything).
		Return(&mongo.UpdateResult{}, nil)
	collection.
		On("CountDocuments", mock.Anything, bson.M{"_id": "rg"}, mock.Anything).
		Return(int64(0), nil)

	_, err := async.Await(repo.Save(ctx, testModel{Name: "Rick", PhotoUrl: "rick.png", Email: "rick@gmail.com"}))
	require.NoError(t, err)
	collection.AssertExpectations(t)
}

func TestSave_Err(t *testing.T) {
	ctx := context.Background()

	collection := &MockCollection{}
	baseRepo := odmCollection[testModel]{col: collection, timer: &MockTimer{}}
	repo := &testOdmCollection{baseRepo}

	expectedErr := fmt.Errorf("failed to save")

	collection.
		On("UpdateOne", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&mongo.UpdateResult{}, expectedErr)
	collection.
		On("CountDocuments", mock.Anything, mock.Anything, mock.Anything).
		Return(int64(0), nil)

	_, err := async.Await(repo.Save(ctx, testModel{Name: "Rick"}))
	require.ErrorIs(t, err, expectedErr)
}

func TestSave_Update(t *testing.T) {
	ctx := context.Background()

	collection := &MockCollection{}
	baseRepo := odmCollection[testModel]{col: collection, timer: &MockTimer{}}
	repo := &testOdmCollection{baseRepo}

	expectedFilter := bson.M{"_id": "rg"}
	expectedUpdate := bson.M{
		"$set": bson.M{
			"_id":       "rg",
			"name":      "Rick",
			"photoUrl":  "rick.png",
			"email":     "rick@gmail.com",
			"updatedOn": int64(2024),
		},
	}

	collection.
		On("UpdateOne", mock.Anything, expectedFilter, expectedUpdate, mock.Anything).
		Return(&mongo.UpdateResult{}, nil)
	collection.
		On("CountDocuments", mock.Anything, bson.M{"_id": "rg"}, mock.Anything).
		Return(int64(1), nil)

	_, err := async.Await(repo.Save(ctx, testModel{Name: "Rick", PhotoUrl: "rick.png", Email: "rick@gmail.com"}))
	require.NoError(t, err)
	collection.AssertExpectations(t)
}

func TestFindOneByID(t *testing.T) {
	ctx := context.Background()

	collection := &MockCollection{}
	baseRepo := odmCollection[testModel]{col: collection, timer: &MockTimer{}}
	repo := &testOdmCollection{baseRepo}

	expectedFilter := bson.M{"_id": "rg"}
	expectedModel := &testModel{Name: "Rick", PhotoUrl: "rick.png", Email: "rick@gmail.com"}

	findOneResult := mongo.NewSingleResultFromDocument(expectedModel, nil, nil)
	collection.On("FindOne", mock.Anything, expectedFilter, mock.Anything).
		Return(findOneResult)

	res, err := async.Await(repo.FindOneByID(ctx, "rg"))
	require.NoError(t, err)
	require.Equal(t, expectedModel, res)
}

func TestFind(t *testing.T) {
	ctx := context.Background()

	collection := &MockCollection{}
	baseRepo := odmCollection[testModel]{col: collection, timer: &MockTimer{}}
	repo := &testOdmCollection{baseRepo}

	filter := bson.M{"email": "rick@gmail.com"}
	sort := bson.D{{Key: "name", Value: 1}}
	limit, skip := int64(10), int64(0)

	expected := []testModel{{Name: "Rick", Email: "rick@gmail.com"}}
	cursor, _ := mongo.NewCursorFromDocuments(toInterface(expected), nil, nil)

	collection.On("Find", mock.Anything, filter, mock.Anything).
		Return(cursor, nil)

	res, err := async.Await(repo.Find(ctx, filter, sort, limit, skip))
	require.NoError(t, err)
	require.Equal(t, expected, res)
}

func TestAggregate(t *testing.T) {
	ctx := context.Background()

	collection := &MockCollection{}
	baseRepo := odmCollection[testModel]{col: collection, timer: &MockTimer{}}
	repo := &testOdmCollection{baseRepo}

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{{Key: "Name", Value: "Rick"}}}},
		bson.D{{Key: "$sample", Value: bson.D{{Key: "size", Value: 1}}}},
	}

	expected := []testModel{
		{Name: "Rick", PhotoUrl: "rick.png", Email: "rick.ag@gmail.com"},
		{Name: "Rick", PhotoUrl: "rickPt.png", Email: "rick.pt@gmail.com"},
	}

	cursor, _ := mongo.NewCursorFromDocuments(toInterface(expected), nil, nil)
	collection.On("Aggregate", mock.Anything, pipeline, mock.Anything).
		Return(cursor, nil)

	res, err := async.Await(repo.Aggregate(ctx, pipeline))
	require.NoError(t, err)
	require.Equal(t, expected, res)
}

func TestDeleteByID(t *testing.T) {
	ctx := context.Background()

	collection := &MockCollection{}
	baseRepo := odmCollection[testModel]{col: collection, timer: &MockTimer{}}
	repo := &testOdmCollection{baseRepo}

	expectedFilter := bson.M{"_id": "rg"}
	collection.On("DeleteOne", mock.Anything, expectedFilter, mock.Anything).
		Return(&mongo.DeleteResult{DeletedCount: 1}, nil)

	_, err := async.Await(repo.DeleteByID(ctx, "rg"))
	require.NoError(t, err)
}

func TestDeleteOne(t *testing.T) {
	ctx := context.Background()

	collection := &MockCollection{}
	baseRepo := odmCollection[testModel]{col: collection, timer: &MockTimer{}}
	repo := &testOdmCollection{baseRepo}

	filter := bson.M{"email": "rick@gmail.com"}
	collection.On("DeleteOne", mock.Anything, filter, mock.Anything).
		Return(&mongo.DeleteResult{DeletedCount: 1}, nil)

	_, err := async.Await(repo.DeleteOne(ctx, filter))
	require.NoError(t, err)
}

func TestCountDocuments(t *testing.T) {
	ctx := context.Background()

	collection := &MockCollection{}
	baseRepo := odmCollection[testModel]{col: collection, timer: &MockTimer{}}
	repo := &testOdmCollection{baseRepo}

	filter := bson.M{"email": "rick@gmail.com"}
	collection.On("CountDocuments", mock.Anything, filter, mock.Anything).
		Return(int64(5), nil)

	count, err := async.Await(repo.Count(ctx, filter))
	require.NoError(t, err)
	require.Equal(t, int64(5), count)
}

func TestDistinct(t *testing.T) {
	ctx := context.Background()

	collection := &MockCollection{}
	baseRepo := odmCollection[testModel]{col: collection, timer: &MockTimer{}}
	repo := &testOdmCollection{baseRepo}

	field := "email"
	filter := bson.D{}
	expected := []interface{}{"rick@gmail.com", "rick@foo.com"}

	collection.On("Distinct", mock.Anything, field, filter, mock.Anything).
		Return(expected, nil)

	res, err := async.Await(repo.Distinct(ctx, field, filter, time.Second))
	require.NoError(t, err)
	require.Equal(t, expected, res)
}

func TestExists(t *testing.T) {
	ctx := context.Background()

	collection := &MockCollection{}
	baseRepo := odmCollection[testModel]{col: collection, timer: &MockTimer{}}
	repo := &testOdmCollection{baseRepo}

	collection.On("CountDocuments", mock.Anything, bson.M{"_id": "rg"}, mock.Anything).
		Return(int64(1), nil)

	exists, err := async.Await(repo.Exists(ctx, "rg"))
	require.NoError(t, err)
	require.True(t, exists)
}

/* ─────────────────────────────
   Helpers & mocks
   ───────────────────────────── */

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

func (m *MockCollection) Find(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error) {
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

func (m *MockCollection) Distinct(ctx context.Context, field string, filter interface{}, opts ...*options.DistinctOptions) ([]interface{}, error) {
	args := m.Called(ctx, field, filter, opts)
	return args.Get(0).([]interface{}), args.Error(1)
}

func toInterface(models []testModel) []interface{} {
	out := make([]interface{}, len(models))
	for i, m := range models {
		out[i] = m
	}
	return out
}
