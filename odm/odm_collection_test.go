package odm

import (
	"context"
	"fmt"
	"testing"

	"github.com/SaiNageswarS/go-api-boot/async"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
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
	assert.NoError(t, err)
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
	assert.ErrorIs(t, err, expectedErr)
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
	assert.NoError(t, err)
	collection.AssertExpectations(t)
}

func TestSave_ErrBsonConvert(t *testing.T) {
	restore := convertToBson
	convertToBson = func(model DbModel) (bson.M, error) { return nil, fmt.Errorf("bson conversion error") }
	t.Cleanup(func() { convertToBson = restore })

	ctx := context.Background()

	collection := &MockCollection{}
	baseRepo := odmCollection[testModel]{col: collection, timer: &MockTimer{}}
	repo := &testOdmCollection{baseRepo}

	_, err := async.Await(repo.Save(ctx, testModel{Name: "Rick"}))
	assert.Error(t, err)
	assert.EqualError(t, err, "bson conversion error")
	collection.AssertNotCalled(t, "UpdateOne")
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
	assert.NoError(t, err)
	assert.Equal(t, expectedModel, res)
}

func TestFindOneById_Err(t *testing.T) {
	ctx := context.Background()

	collection := &MockCollection{}
	baseRepo := odmCollection[testModel]{col: collection, timer: &MockTimer{}}
	repo := &testOdmCollection{baseRepo}

	findOneErrResult := mongo.NewSingleResultFromDocument(nil, fmt.Errorf("not found"), nil)
	collection.On("FindOne", mock.Anything, mock.Anything, mock.Anything).
		Return(findOneErrResult)

	res, err := async.Await(repo.FindOneByID(ctx, "rg"))
	assert.Error(t, err)
	assert.Nil(t, res)
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
	assert.NoError(t, err)
	assert.Equal(t, expected, res)
}

func TestFind_Err(t *testing.T) {
	ctx := context.Background()

	collection := &MockCollection{}
	baseRepo := odmCollection[testModel]{col: collection, timer: &MockTimer{}}
	repo := &testOdmCollection{baseRepo}

	cursor, _ := mongo.NewCursorFromDocuments(toInterface([]testModel{}), nil, nil)
	collection.On("Find", mock.Anything, mock.Anything, mock.Anything).
		Return(cursor, fmt.Errorf("find error"))

	_, err := async.Await(repo.Find(ctx, bson.M{"email": "rick@gmail.com"}, bson.D{{Key: "name", Value: 1}}, int64(10), int64(10)))
	assert.Error(t, err)
	assert.EqualError(t, err, "find error")
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
	assert.NoError(t, err)
	assert.Equal(t, expected, res)
}

func TestAggregate_Err(t *testing.T) {
	ctx := context.Background()

	collection := &MockCollection{}
	baseRepo := odmCollection[testModel]{col: collection, timer: &MockTimer{}}
	repo := &testOdmCollection{baseRepo}

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{{Key: "Name", Value: "Rick"}}}},
		bson.D{{Key: "$sample", Value: bson.D{{Key: "size", Value: 1}}}},
	}

	cursor, _ := mongo.NewCursorFromDocuments(toInterface([]testModel{}), nil, nil)
	collection.On("Aggregate", mock.Anything, pipeline, mock.Anything).
		Return(cursor, fmt.Errorf("aggregation error"))

	_, err := async.Await(repo.Aggregate(ctx, pipeline))
	assert.Error(t, err)
	assert.EqualError(t, err, "aggregation error")
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
	assert.NoError(t, err)
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
	assert.NoError(t, err)
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
	assert.NoError(t, err)
	assert.Equal(t, int64(5), count)
}

func TestExists(t *testing.T) {
	ctx := context.Background()

	collection := &MockCollection{}
	baseRepo := odmCollection[testModel]{col: collection, timer: &MockTimer{}}
	repo := &testOdmCollection{baseRepo}

	collection.On("CountDocuments", mock.Anything, bson.M{"_id": "rg"}, mock.Anything).
		Return(int64(1), nil)

	exists, err := async.Await(repo.Exists(ctx, "rg"))
	assert.NoError(t, err)
	assert.True(t, exists)
}

func TestExists_Err(t *testing.T) {
	ctx := context.Background()

	collection := &MockCollection{}
	baseRepo := odmCollection[testModel]{col: collection, timer: &MockTimer{}}
	repo := &testOdmCollection{baseRepo}

	collection.On("CountDocuments", mock.Anything, bson.M{"_id": "rg"}, mock.Anything).
		Return(int64(0), fmt.Errorf("count error"))

	exists, err := async.Await(repo.Exists(ctx, "rg"))
	assert.Error(t, err)
	assert.False(t, exists)
}

func TestVectorSearch_Success(t *testing.T) {
	ctx := context.Background()

	collection := &MockCollection{}
	baseRepo := odmCollection[testModel]{col: collection, timer: &MockTimer{}}
	repo := &testOdmCollection{baseRepo}

	query := VectorSearchParams{
		IndexName:     "test_vector_index",
		Path:          "embedding",
		K:             5,
		NumCandidates: 200,
	}

	expected := []SearchHit[testModel]{{Doc: testModel{Name: "Rick", Email: "rick@gmail.com"}, Score: 0.8}}
	cursor, _ := mongo.NewCursorFromDocuments(toInterface(expected), nil, nil)
	collection.On("Aggregate", mock.Anything, mock.Anything, mock.Anything).
		Return(cursor, nil)

	res, err := async.Await(repo.VectorSearch(ctx, []float32{0.1, 0.2, 0.3}, query))
	assert.NoError(t, err)
	assert.Equal(t, expected, res)
}

func TestVectorSearch_Err(t *testing.T) {
	ctx := context.Background()
	collection := &MockCollection{}
	baseRepo := odmCollection[testModel]{col: collection, timer: &MockTimer{}}
	repo := &testOdmCollection{baseRepo}
	query := VectorSearchParams{
		IndexName:     "test_vector_index",
		Path:          "embedding",
		K:             5,
		NumCandidates: 200,
	}

	cursor, _ := mongo.NewCursorFromDocuments(toInterface([]testModel{}), nil, nil)
	collection.On("Aggregate", mock.Anything, mock.Anything, mock.Anything).
		Return(cursor, fmt.Errorf("aggregation error"))

	_, err := async.Await(repo.VectorSearch(ctx, []float32{0.1, 0.2, 0.3}, query))
	assert.Error(t, err)
	assert.EqualError(t, err, "aggregation error")
}

/* ─────────────────────────────
   Helpers & mocks
   ───────────────────────────── */

type MockCollection struct {
	mock.Mock
}

func (m *MockCollection) UpdateOne(ctx context.Context, filter interface{}, update interface{}, opts ...options.Lister[options.UpdateOneOptions]) (*mongo.UpdateResult, error) {
	args := m.Called(ctx, filter, update, opts)
	return args.Get(0).(*mongo.UpdateResult), args.Error(1)
}

func (m *MockCollection) FindOne(ctx context.Context, filter interface{}, opts ...options.Lister[options.FindOneOptions]) *mongo.SingleResult {
	args := m.Called(ctx, filter, opts)
	return args.Get(0).(*mongo.SingleResult)
}

func (m *MockCollection) Find(ctx context.Context, filter interface{}, opts ...options.Lister[options.FindOptions]) (*mongo.Cursor, error) {
	args := m.Called(ctx, filter, opts)
	return args.Get(0).(*mongo.Cursor), args.Error(1)
}

func (m *MockCollection) DeleteOne(ctx context.Context, filter interface{}, opts ...options.Lister[options.DeleteOneOptions]) (*mongo.DeleteResult, error) {
	args := m.Called(ctx, filter, opts)
	return args.Get(0).(*mongo.DeleteResult), args.Error(1)
}

func (m *MockCollection) Aggregate(ctx context.Context, pipeline interface{}, opts ...options.Lister[options.AggregateOptions]) (*mongo.Cursor, error) {
	args := m.Called(ctx, pipeline, opts)
	return args.Get(0).(*mongo.Cursor), args.Error(1)
}

func (m *MockCollection) CountDocuments(ctx context.Context, filter interface{}, opts ...options.Lister[options.CountOptions]) (int64, error) {
	args := m.Called(ctx, filter, opts)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockCollection) Distinct(ctx context.Context, field string, filter interface{}, opts ...options.Lister[options.DistinctOptions]) *mongo.DistinctResult {
	args := m.Called(ctx, field, filter, opts)
	return args.Get(0).(*mongo.DistinctResult)
}

func toInterface[T any](models []T) []interface{} {
	out := make([]interface{}, len(models))
	for i, m := range models {
		out[i] = m
	}
	return out
}
