package mongodb

import (
	"context"
	"errors"
	"fmt"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-api-boot/odm/internal/core"
	"github.com/SaiNageswarS/go-collection-boot/async"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Collection is a typed MongoDB collection of T in one tenant (a database).
// Filters are MongoDB query documents.
//
// Every method runs asynchronously and delivers one async.Result, so
// independent queries run concurrently.
type Collection[T odm.Model] interface {
	// Save upserts model: its fields replace the stored ones, fields absent
	// from model are kept. It stamps createdOn on insert and updatedOn on
	// every save, atomically (see odm.Timestamps).
	Save(ctx context.Context, model T) <-chan async.Result[struct{}]

	// FindByID returns the document with id, or odm.ErrNotFound.
	FindByID(ctx context.Context, id string) <-chan async.Result[*T]
	// FindOne returns a document matching filter, or odm.ErrNotFound. A nil
	// filter matches every document.
	FindOne(ctx context.Context, filter bson.M) <-chan async.Result[*T]
	// Find returns the documents matching filter. A nil filter matches all.
	Find(ctx context.Context, filter bson.M, opts ...FindOption) <-chan async.Result[[]T]
	// Count counts the documents matching filter.
	Count(ctx context.Context, filter bson.M) <-chan async.Result[int64]
	// Exists reports whether a document with id exists.
	Exists(ctx context.Context, id string) <-chan async.Result[bool]
	// Distinct returns the distinct values of field among documents matching
	// filter; array fields contribute their elements. Use DistinctAs for
	// typed values.
	Distinct(ctx context.Context, field string, filter bson.M) <-chan async.Result[[]any]

	// DeleteByID deletes the document with id, if any.
	DeleteByID(ctx context.Context, id string) <-chan async.Result[struct{}]
	// DeleteOne deletes one document matching filter, if any. filter must
	// not be nil.
	DeleteOne(ctx context.Context, filter bson.M) <-chan async.Result[struct{}]

	// VectorSearch runs Atlas Vector Search on index, one of the model's
	// VectorIndexSpecs.
	VectorSearch(ctx context.Context, index odm.VectorIndexSpec, embedding []float32, params VectorSearchParams) <-chan async.Result[[]odm.SearchHit[T]]
	// TermSearch runs Atlas Search (any of the query's words) over the paths
	// of index, one of the model's TermSearchIndexSpecs.
	TermSearch(ctx context.Context, index odm.TermSearchIndexSpec, query string, params TermSearchParams) <-chan async.Result[[]odm.SearchHit[T]]

	// Aggregate runs pipeline and decodes each output document into T.
	Aggregate(ctx context.Context, pipeline mongo.Pipeline) <-chan async.Result[[]T]
	// Raw returns the driver collection, for anything the ODM doesn't cover.
	Raw() *mongo.Collection
}

var _ Collection[odm.Model] = (*collection[odm.Model])(nil)

// The fields Save stamps; odm.Timestamps maps onto them.
const (
	createdOnField = "createdOn"
	updatedOnField = "updatedOn"
)

type collection[T odm.Model] struct {
	col    *mongo.Collection
	timer  odm.Timer
	target string // "<tenant>.<collection>", for errors
}

// CollectionOf returns the collection of T in tenant (a MongoDB database).
func CollectionOf[T odm.Model](c *Client, tenant string) Collection[T] {
	var zero T
	name := zero.CollectionName()
	return &collection[T]{col: c.collection(tenant, name), timer: c.timer, target: tenant + "." + name}
}

func (c *collection[T]) Raw() *mongo.Collection { return c.col }

// Save upserts in one atomic update: model's fields and updatedOn are $set,
// createdOn is set only on insert.
func (c *collection[T]) Save(ctx context.Context, model T) <-chan async.Result[struct{}] {
	return async.Go(func() (_ struct{}, err error) {
		defer annotate(&err, c.target)
		id := model.Id()
		if err := core.CheckID(id); err != nil {
			return struct{}{}, err
		}
		doc, err := toDoc(model)
		if err != nil {
			return struct{}{}, err
		}
		delete(doc, "_id")
		delete(doc, createdOnField)
		now := c.timer.Now()
		doc[updatedOnField] = now

		_, err = c.col.UpdateOne(ctx,
			bson.M{"_id": id},
			bson.M{"$set": doc, "$setOnInsert": bson.M{createdOnField: now}},
			options.UpdateOne().SetUpsert(true))
		return struct{}{}, err
	})
}

func (c *collection[T]) FindByID(ctx context.Context, id string) <-chan async.Result[*T] {
	return c.FindOne(ctx, bson.M{"_id": id})
}

func (c *collection[T]) FindOne(ctx context.Context, filter bson.M) <-chan async.Result[*T] {
	return async.Go(func() (_ *T, err error) {
		defer annotate(&err, c.target)
		res := c.col.FindOne(ctx, filterDoc(filter))
		if err := res.Err(); err != nil {
			return nil, err
		}
		model := new(T)
		return model, res.Decode(model)
	})
}

func (c *collection[T]) Find(ctx context.Context, filter bson.M, opts ...FindOption) <-chan async.Result[[]T] {
	return async.Go(func() (_ []T, err error) {
		defer annotate(&err, c.target)
		o, err := resolveFindOptions(opts...)
		if err != nil {
			return nil, err
		}
		cursor, err := c.col.Find(ctx, filterDoc(filter), o.driverOptions())
		if err != nil {
			return nil, err
		}
		return decodeAll[T](ctx, cursor)
	})
}

func (c *collection[T]) Count(ctx context.Context, filter bson.M) <-chan async.Result[int64] {
	return async.Go(func() (_ int64, err error) {
		defer annotate(&err, c.target)
		return c.col.CountDocuments(ctx, filterDoc(filter))
	})
}

func (c *collection[T]) Exists(ctx context.Context, id string) <-chan async.Result[bool] {
	return async.Go(func() (_ bool, err error) {
		defer annotate(&err, c.target)
		n, err := c.col.CountDocuments(ctx, bson.M{"_id": id}, options.Count().SetLimit(1))
		return n > 0, err
	})
}

func (c *collection[T]) Distinct(ctx context.Context, field string, filter bson.M) <-chan async.Result[[]any] {
	return async.Go(func() (_ []any, err error) {
		defer annotate(&err, c.target)
		res := c.col.Distinct(ctx, field, filterDoc(filter))
		if err := res.Err(); err != nil {
			return nil, err
		}
		values := []any{}
		return values, res.Decode(&values)
	})
}

func (c *collection[T]) DeleteByID(ctx context.Context, id string) <-chan async.Result[struct{}] {
	return c.DeleteOne(ctx, bson.M{"_id": id})
}

func (c *collection[T]) DeleteOne(ctx context.Context, filter bson.M) <-chan async.Result[struct{}] {
	return async.Go(func() (_ struct{}, err error) {
		defer annotate(&err, c.target)
		if filter == nil {
			return struct{}{}, errors.New("DeleteOne needs a filter")
		}
		_, err = c.col.DeleteOne(ctx, filter)
		return struct{}{}, err
	})
}

func (c *collection[T]) Aggregate(ctx context.Context, pipeline mongo.Pipeline) <-chan async.Result[[]T] {
	return async.Go(func() (_ []T, err error) {
		defer annotate(&err, c.target)
		cursor, err := c.col.Aggregate(ctx, pipeline)
		if err != nil {
			return nil, err
		}
		return decodeAll[T](ctx, cursor)
	})
}

func (c *collection[T]) VectorSearch(ctx context.Context, index odm.VectorIndexSpec, embedding []float32, params VectorSearchParams) <-chan async.Result[[]odm.SearchHit[T]] {
	return async.Go(func() (_ []odm.SearchHit[T], err error) {
		defer annotate(&err, c.target)
		pipeline, err := vectorSearchPipeline(index, embedding, params)
		if err != nil {
			return nil, err
		}
		cursor, err := c.col.Aggregate(ctx, pipeline)
		if err != nil {
			return nil, err
		}
		return decodeAll[odm.SearchHit[T]](ctx, cursor)
	})
}

func (c *collection[T]) TermSearch(ctx context.Context, index odm.TermSearchIndexSpec, query string, params TermSearchParams) <-chan async.Result[[]odm.SearchHit[T]] {
	return async.Go(func() (_ []odm.SearchHit[T], err error) {
		defer annotate(&err, c.target)
		pipeline, err := termSearchPipeline(index, query, params)
		if err != nil {
			return nil, err
		}
		cursor, err := c.col.Aggregate(ctx, pipeline)
		if err != nil {
			return nil, err
		}
		return decodeAll[odm.SearchHit[T]](ctx, cursor)
	})
}

// DistinctAs is Distinct with values decoded into V, e.g.
// mongodb.DistinctAs[string](ctx, movies, "genres", nil).
func DistinctAs[V any, T odm.Model](ctx context.Context, c Collection[T], field string, filter bson.M) <-chan async.Result[[]V] {
	return async.Go(func() ([]V, error) {
		values, err := async.Await(c.Distinct(ctx, field, filter))
		if err != nil {
			return nil, err // already annotated by Distinct
		}
		raw, err := bson.Marshal(bson.D{{Key: "v", Value: bson.A(values)}})
		if err != nil {
			return nil, err
		}
		var out struct {
			V []V `bson:"v"`
		}
		if err := bson.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("odm/mongodb: decoding distinct %s values: %w", field, err)
		}
		if out.V == nil {
			out.V = []V{}
		}
		return out.V, nil
	})
}

func decodeAll[R any](ctx context.Context, cursor *mongo.Cursor) ([]R, error) {
	out := []R{}
	return out, cursor.All(ctx, &out)
}

// toDoc marshals model into a document.
func toDoc(model any) (bson.M, error) {
	raw, err := bson.Marshal(model)
	if err != nil {
		return nil, err
	}
	var doc bson.M
	return doc, bson.Unmarshal(raw, &doc)
}
