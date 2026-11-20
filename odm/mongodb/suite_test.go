// The MongoDB backend's integration suite: CRUD, upsert and timestamp
// semantics, filters, sorting, pagination, distinct, duplicate keys,
// not-found, missing collections, crash safety, and Atlas Search and Vector
// Search.

package mongodb

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Person is a sub-document with its own _id.
type Person struct {
	ID   string `bson:"_id"`
	Name string `bson:"name"`
}

// Movie is the suite's model.
type Movie struct {
	ID        string      `bson:"_id"`
	Title     string      `bson:"title"`
	Plot      string      `bson:"plot"`
	Year      int         `bson:"year"`
	Genres    []string    `bson:"genres"`
	Cast      []Person    `bson:"cast"`
	Rating    float64     `bson:"rating"`
	Released  time.Time   `bson:"released"`
	Note      string      `bson:"note,omitempty"`
	Embedding bson.Vector `bson:"embedding"`
	CreatedOn int64       `bson:"createdOn,omitempty"`
	UpdatedOn int64       `bson:"updatedOn,omitempty"`
}

func (m Movie) Id() string             { return m.ID }
func (m Movie) CollectionName() string { return "movies" }

func (m Movie) Indexes() []odm.IndexSpec {
	return []odm.IndexSpec{
		{Keys: []odm.IndexKey{odm.Key("title")}, Unique: true},
		{Name: "byYear", Keys: []odm.IndexKey{odm.DescKey("year"), odm.Key("rating")}},
	}
}

func (m Movie) TermSearchIndexes() []odm.TermSearchIndexSpec {
	return []odm.TermSearchIndexSpec{{Name: "plotIndex", Paths: []string{"title", "plot"}}}
}

func (m Movie) VectorIndexes() []odm.VectorIndexSpec {
	return []odm.VectorIndexSpec{{
		Name: "embeddingIndex", Path: "embedding", Dimensions: 3,
		Similarity: odm.Cosine, FilterPaths: []string{"year"},
	}}
}

// clock is a settable odm.Timer.
type clock struct{ now atomic.Int64 }

func (c *clock) Now() int64     { return c.now.Load() }
func (c *clock) Set(unix int64) { c.now.Store(unix) }

// backend adapts the client under test to the suite.
type backend struct {
	// Clock must be the Timer of the client Collection uses.
	Clock *clock
	// Collection returns the Movie collection of tenant.
	Collection func(tenant string) Collection[Movie]
	// Ensure runs the backend's EnsureCollection[Movie].
	Ensure func(ctx context.Context, tenant string) error
	// Drop deletes tenant.
	Drop func(ctx context.Context, tenant string)
	// SearchLag is how long search results may take to reflect writes
	// (Atlas Search indexes asynchronously); 0 for immediate.
	SearchLag time.Duration
}

func date(year int) time.Time { return time.Date(year, 6, 1, 0, 0, 0, 0, time.UTC) }

// fixtures are the suite's documents.
func fixtures() []Movie {
	return []Movie{
		{ID: "m1", Title: "Alien", Plot: "A crew aboard a spaceship meets a deadly alien creature.", Year: 1979,
			Genres: []string{"horror", "sci-fi"}, Cast: []Person{{"p1", "Sigourney"}, {"p2", "Tom"}}, Rating: 8.5,
			Released: date(1965), Embedding: bson.NewVector([]float32{1, 0, 0})},
		{ID: "m2", Title: "Aliens", Plot: "Marines fight aliens on a distant colony.", Year: 1986,
			Genres: []string{"action", "sci-fi"}, Cast: []Person{{"p1", "Sigourney"}}, Rating: 8.4,
			Released: date(1985), Embedding: bson.NewVector([]float32{0.9, 0.1, 0})},
		{ID: "m3", Title: "Heat", Plot: "A detective hunts\na crew of bank robbers in Los Angeles.", Year: 1995,
			Genres: []string{"crime"}, Cast: []Person{{"p3", "Al"}}, Rating: 8.3,
			Released: date(1995), Embedding: bson.NewVector([]float32{0, 1, 0})},
		{ID: "m4", Title: "Ronin", Plot: "Mercenaries chase a mysterious case across France.", Year: 1998,
			Genres: []string{"action", "crime"}, Cast: []Person{{"p4", "Robert"}}, Rating: 7.2,
			Released: date(1998), Embedding: bson.NewVector([]float32{0, 0.2, 1})},
	}
}

// runSuite runs the suite. tenant must be unique to this run.
func runSuite(t *testing.T, b backend, tenant string) {
	ctx := t.Context()
	t.Cleanup(func() { b.Drop(context.Background(), tenant) })

	t.Run("reads of a collection that doesn't exist find nothing", func(t *testing.T) {
		col := b.Collection(tenant + "_missing")
		_, err := async.Await(col.FindByID(ctx, "m1"))
		assert.ErrorIs(t, err, odm.ErrNotFound)
		docs, err := async.Await(col.Find(ctx, nil))
		require.NoError(t, err)
		assert.Empty(t, docs)
		n, err := async.Await(col.Count(ctx, nil))
		require.NoError(t, err)
		assert.Zero(t, n)
		ok, err := async.Await(col.Exists(ctx, "m1"))
		require.NoError(t, err)
		assert.False(t, ok)
	})

	require.NoError(t, b.Ensure(ctx, tenant))
	require.NoError(t, b.Ensure(ctx, tenant), "EnsureCollection must be idempotent")

	col := b.Collection(tenant)
	b.Clock.Set(100)
	for _, m := range fixtures() {
		_, err := async.Await(col.Save(ctx, m))
		require.NoError(t, err)
	}

	t.Run("Save stamps and merges", func(t *testing.T) {
		m, err := async.Await(col.FindByID(ctx, "m1"))
		require.NoError(t, err)
		assert.Equal(t, "Alien", m.Title)
		assert.Equal(t, []Person{{"p1", "Sigourney"}, {"p2", "Tom"}}, m.Cast)
		assert.True(t, date(1965).Equal(m.Released))
		f, _ := m.Embedding.Float32OK()
		assert.Equal(t, []float32{1, 0, 0}, f)
		assert.Equal(t, int64(100), m.CreatedOn)
		assert.Equal(t, int64(100), m.UpdatedOn)

		withNote := fixtures()[0]
		withNote.Note = "classic"
		b.Clock.Set(200)
		_, err = async.Await(col.Save(ctx, withNote))
		require.NoError(t, err)

		// Saving without the omitempty field keeps it: Save is an upsert that
		// merges top-level fields.
		b.Clock.Set(300)
		_, err = async.Await(col.Save(ctx, fixtures()[0]))
		require.NoError(t, err)

		m, err = async.Await(col.FindByID(ctx, "m1"))
		require.NoError(t, err)
		assert.Equal(t, "classic", m.Note)
		assert.Equal(t, int64(100), m.CreatedOn, "createdOn is kept")
		assert.Equal(t, int64(300), m.UpdatedOn)
	})

	t.Run("Save rejects bad models without crashing", func(t *testing.T) {
		_, err := async.Await(col.Save(ctx, Movie{Title: "No id"}))
		assert.Error(t, err)
		// A zero bson.Vector makes the bson encoder panic.
		_, err = async.Await(col.Save(ctx, Movie{ID: "zero-vector", Title: "Zero"}))
		assert.Error(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := async.Await(col.FindByID(ctx, "nope"))
		assert.ErrorIs(t, err, odm.ErrNotFound)
		_, err = async.Await(col.FindOne(ctx, bson.M{"year": 1900}))
		assert.ErrorIs(t, err, odm.ErrNotFound)
	})

	t.Run("duplicate key", func(t *testing.T) {
		dup := fixtures()[2]
		dup.ID = "dup"
		_, err := async.Await(col.Save(ctx, dup))
		assert.ErrorIs(t, err, odm.ErrDuplicateKey)
	})

	t.Run("Find", func(t *testing.T) {
		ids := func(t *testing.T, filter bson.M, opts ...FindOption) []string {
			t.Helper()
			res, err := async.Await(col.Find(ctx, filter, opts...))
			require.NoError(t, err, "%v", filter)
			out := []string{}
			for _, m := range res {
				out = append(out, m.ID)
			}
			return out
		}
		byYear := WithSort(Asc("year"))

		cases := []struct {
			name   string
			filter bson.M
			want   []string
		}{
			{"all", nil, []string{"m1", "m2", "m3", "m4"}},
			{"equality", bson.M{"year": 1986}, []string{"m2"}},
			{"array membership", bson.M{"genres": "sci-fi"}, []string{"m1", "m2"}},
			{"range", bson.M{"year": bson.M{"$gt": 1986, "$lte": 1998}}, []string{"m3", "m4"}},
			{"float range", bson.M{"rating": bson.M{"$gte": 8.4}}, []string{"m1", "m2"}},
			{"dates before 1970", bson.M{"released": bson.M{"$lt": date(1990)}}, []string{"m1", "m2"}},
			{"in", bson.M{"genres": bson.M{"$in": bson.A{"action"}}}, []string{"m2", "m4"}},
			{"nin", bson.M{"genres": bson.M{"$nin": bson.A{"action", "sci-fi"}}}, []string{"m3"}},
			{"id in", bson.M{"_id": bson.M{"$in": bson.A{"m1", "m3"}}}, []string{"m1", "m3"}},
			{"ne", bson.M{"_id": bson.M{"$ne": "m2"}}, []string{"m1", "m3", "m4"}},
			{"exists", bson.M{"note": bson.M{"$exists": true}}, []string{"m1"}},
			{"null matches missing", bson.M{"note": nil}, []string{"m2", "m3", "m4"}},
			{"size", bson.M{"genres": bson.M{"$size": 1}}, []string{"m3"}},
			{"all", bson.M{"genres": bson.M{"$all": bson.A{"crime", "action"}}}, []string{"m4"}},
			{"or", bson.M{"$or": bson.A{bson.M{"year": 1979}, bson.M{"title": "Ronin"}}}, []string{"m1", "m4"}},
			{"nor", bson.M{"$nor": bson.A{bson.M{"year": 1979}, bson.M{"title": "Ronin"}}}, []string{"m2", "m3"}},
			{"not", bson.M{"year": bson.M{"$not": bson.M{"$gt": 1990}}}, []string{"m1", "m2"}},
			{"elemMatch scalar", bson.M{"genres": bson.M{"$elemMatch": bson.M{"$eq": "crime"}}}, []string{"m3", "m4"}},
			{"elemMatch _id is the element's", bson.M{"cast": bson.M{"$elemMatch": bson.M{"_id": "p2"}}}, []string{"m1"}},
			{"elemMatch _id not the row's", bson.M{"cast": bson.M{"$elemMatch": bson.M{"_id": "m1"}}}, []string{}},
			{"dotted path in array", bson.M{"cast.name": "Sigourney"}, []string{"m1", "m2"}},
			{"regex i", bson.M{"title": bson.M{"$regex": "^ali", "$options": "i"}}, []string{"m1", "m2"}},
			{"regex dot stops at newline", bson.M{"plot": bson.M{"$regex": "hunts.a crew"}}, []string{}},
			{"regex s", bson.M{"plot": bson.M{"$regex": "hunts.a crew", "$options": "s"}}, []string{"m3"}},
			{"regex anchors whole string", bson.M{"plot": bson.M{"$regex": "^a crew"}}, []string{}},
			{"regex m", bson.M{"plot": bson.M{"$regex": "^a crew", "$options": "m"}}, []string{"m3"}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				assert.Equal(t, tc.want, ids(t, tc.filter, byYear))
			})
		}

		assert.Equal(t, []string{"m4", "m3", "m2", "m1"}, ids(t, nil, WithSort(Desc("year"))))
		assert.Equal(t, []string{"m1", "m2", "m3", "m4"}, ids(t, nil, WithSort(Asc("released"))))
		assert.Equal(t, []string{"m2", "m3"}, ids(t, nil, byYear, WithLimit(2), WithSkip(1)))

		_, err := async.Await(col.Find(ctx, nil, WithLimit(-1)))
		assert.Error(t, err)
	})

	t.Run("Count, Exists, Distinct", func(t *testing.T) {
		n, err := async.Await(col.Count(ctx, bson.M{"genres": "crime"}))
		require.NoError(t, err)
		assert.Equal(t, int64(2), n)

		ok, err := async.Await(col.Exists(ctx, "m2"))
		require.NoError(t, err)
		assert.True(t, ok)

		genres, err := async.Await(DistinctAs[string](ctx, col, "genres", nil))
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"action", "crime", "horror", "sci-fi"}, genres)

		years, err := async.Await(DistinctAs[int](ctx, col, "year", bson.M{"genres": "crime"}))
		require.NoError(t, err)
		assert.ElementsMatch(t, []int{1995, 1998}, years)
	})

	t.Run("VectorSearch", func(t *testing.T) {
		// A wrong-sized or missing embedding is stored, just not searchable.
		short := Movie{ID: "short", Title: "Short", Year: 2001, Embedding: bson.NewVector([]float32{1, 0})}
		_, err := async.Await(col.Save(ctx, short))
		require.NoError(t, err)
		t.Cleanup(func() { _, _ = async.Await(col.DeleteByID(context.Background(), "short")) })

		eventually(t, b.SearchLag, func(c *assert.CollectT) {
			hits, err := async.Await(col.VectorSearch(ctx, []float32{1, 0.05, 0}, VectorSearchParams{
				IndexName: "embeddingIndex", K: 2, NumCandidates: 50,
			}))
			require.NoError(c, err)
			require.Len(c, hits, 2)
			assert.Equal(c, "m1", hits[0].Doc.ID)
			assert.Equal(c, "m2", hits[1].Doc.ID)
			assert.Greater(c, hits[0].Score, hits[1].Score)
			assert.LessOrEqual(c, hits[0].Score, 1.0)
		})

		eventually(t, b.SearchLag, func(c *assert.CollectT) {
			hits, err := async.Await(col.VectorSearch(ctx, []float32{1, 0, 0}, VectorSearchParams{
				IndexName: "embeddingIndex", K: 2, Filter: bson.M{"year": bson.M{"$gt": 1990}},
			}))
			require.NoError(c, err)
			require.Len(c, hits, 2)
			assert.ElementsMatch(c, []string{"m3", "m4"}, []string{hits[0].Doc.ID, hits[1].Doc.ID})
		})

		_, err = async.Await(col.VectorSearch(ctx, []float32{1, 0, 0}, VectorSearchParams{IndexName: "nope", K: 1}))
		assert.ErrorContains(t, err, "no vector index")
		_, err = async.Await(col.VectorSearch(ctx, []float32{1, 0}, VectorSearchParams{IndexName: "embeddingIndex", K: 1}))
		assert.ErrorContains(t, err, "dimensions")
	})

	t.Run("TermSearch", func(t *testing.T) {
		eventually(t, b.SearchLag, func(c *assert.CollectT) {
			hits, err := async.Await(col.TermSearch(ctx, "crew robbers", TermSearchParams{IndexName: "plotIndex", Limit: 10}))
			require.NoError(c, err)
			require.Len(c, hits, 2, "any word matches")
			assert.Equal(c, "m3", hits[0].Doc.ID, "matching both words ranks first")
			assert.Greater(c, hits[0].Score, hits[1].Score)
		})

		eventually(t, b.SearchLag, func(c *assert.CollectT) {
			hits, err := async.Await(col.TermSearch(ctx, "crew", TermSearchParams{
				IndexName: "plotIndex", Limit: 10, Filter: bson.M{"year": 1979},
			}))
			require.NoError(c, err)
			require.Len(c, hits, 1)
			assert.Equal(c, "m1", hits[0].Doc.ID)
		})

		_, err := async.Await(col.TermSearch(ctx, "crew", TermSearchParams{IndexName: "nope", Limit: 1}))
		assert.ErrorContains(t, err, "no term search index")
	})

	t.Run("Delete", func(t *testing.T) {
		_, err := async.Await(col.DeleteByID(ctx, "m4"))
		require.NoError(t, err)
		_, err = async.Await(col.DeleteOne(ctx, bson.M{"genres": "sci-fi"}))
		require.NoError(t, err)
		n, err := async.Await(col.Count(ctx, nil))
		require.NoError(t, err)
		assert.Equal(t, int64(2), n, "DeleteOne removes a single match")

		_, err = async.Await(col.DeleteOne(ctx, nil))
		assert.Error(t, err)
	})
}

// eventually retries check until it passes, for backends whose search indexes
// lag behind writes.
func eventually(t *testing.T, lag time.Duration, check func(c *assert.CollectT)) {
	t.Helper()
	assert.EventuallyWithT(t, check, max(lag, time.Second), 100*time.Millisecond)
}
