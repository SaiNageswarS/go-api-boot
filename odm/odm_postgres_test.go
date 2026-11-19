// Integration tests for the Postgres ODM. They need a Postgres with pgvector:
//
//	docker run -d -e POSTGRES_PASSWORD=test -p 55432:5432 pgvector/pgvector:pg17
//	POSTGRES_URI=postgres://postgres:test@localhost:55432/postgres go test ./odm -run Postgres
//
// They are skipped when POSTGRES_URI is not set.

package odm

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/SaiNageswarS/go-api-boot/dotenv"
	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type pgMovie struct {
	ID     string   `bson:"_id"`
	Title  string   `bson:"title"`
	Plot   string   `bson:"plot"`
	Year   int      `bson:"year"`
	Genres []string `bson:"genres"`
	Rating struct {
		Imdb float64 `bson:"imdb"`
	} `bson:"rating"`
	Released  time.Time   `bson:"released"`
	Embedding bson.Vector `bson:"embedding"`
	CreatedOn int64       `bson:"createdOn,omitempty"`
	UpdatedOn int64       `bson:"updatedOn,omitempty"`
}

func (m pgMovie) Id() string             { return m.ID }
func (m pgMovie) CollectionName() string { return "movies" }

func (m pgMovie) IndexModels() []mongo.IndexModel {
	return []mongo.IndexModel{
		{Keys: bson.D{{Key: "title", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "year", Value: -1}, {Key: "rating.imdb", Value: 1}}},
	}
}

func (m pgMovie) TermSearchIndexSpecs() []TermSearchIndexSpec {
	return []TermSearchIndexSpec{{Name: "plotIndex", Paths: []string{"title", "plot"}}}
}

func (m pgMovie) VectorIndexSpecs() []VectorIndexSpec {
	return []VectorIndexSpec{
		{Name: "embeddingIndex", Path: "embedding", Type: "vector", NumDimensions: 3, Similarity: "cosine"},
		{Name: "yearFilter", Path: "year", Type: "filter"},
	}
}

type pgTTLModel struct {
	ID string `bson:"_id"`
}

func (m pgTTLModel) Id() string             { return m.ID }
func (m pgTTLModel) CollectionName() string { return "sessions" }
func (m pgTTLModel) IndexModels() []mongo.IndexModel {
	return []mongo.IndexModel{{
		Keys:    bson.D{{Key: "createdOn", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(3600),
	}}
}

func TestPgCollection(t *testing.T) {
	dotenv.LoadEnv("../.env")
	if os.Getenv("POSTGRES_URI") == "" {
		t.Skip("POSTGRES_URI not set")
	}
	ctx := t.Context()
	client := ProvidePgClient()
	defer client.Close()

	tenant := fmt.Sprintf("odm_test_%d", time.Now().UnixNano())
	defer func() {
		_, _ = client.Exec(ctx, "DROP SCHEMA IF EXISTS "+pgx.Identifier{tenant}.Sanitize()+" CASCADE")
	}()

	require.NoError(t, EnsurePgIndexes[pgMovie](ctx, client, tenant))
	require.NoError(t, EnsurePgIndexes[pgMovie](ctx, client, tenant), "must be idempotent")

	col := PgCollectionOf[pgMovie](client, tenant)
	date := func(y int) time.Time { return time.Date(y, 6, 1, 0, 0, 0, 0, time.UTC) }
	movies := []pgMovie{
		{ID: "m1", Title: "Alien", Plot: "A crew aboard a spaceship meets a deadly alien creature.", Year: 1979, Genres: []string{"horror", "sci-fi"}, Released: date(1979), Embedding: bson.NewVector([]float32{1, 0, 0})},
		{ID: "m2", Title: "Aliens", Plot: "Marines fight aliens on a distant colony.", Year: 1986, Genres: []string{"action", "sci-fi"}, Released: date(1986), Embedding: bson.NewVector([]float32{0.9, 0.1, 0})},
		{ID: "m3", Title: "Heat", Plot: "A detective hunts a crew of bank robbers in Los Angeles.", Year: 1995, Genres: []string{"crime"}, Released: date(1995), Embedding: bson.NewVector([]float32{0, 1, 0})},
		{ID: "m4", Title: "Ronin", Plot: "Mercenaries chase a mysterious case across France.", Year: 1998, Genres: []string{"action", "crime"}, Released: date(1998), Embedding: bson.NewVector([]float32{0, 0.2, 1})},
	}
	movies[0].Rating.Imdb, movies[1].Rating.Imdb, movies[2].Rating.Imdb, movies[3].Rating.Imdb = 8.5, 8.4, 8.3, 7.2

	col.(*pgCollection[pgMovie]).timer = &MockTimer{}
	for _, m := range movies {
		_, err := async.Await(col.Save(ctx, m))
		require.NoError(t, err)
	}

	t.Run("FindOneByID and Save stamps", func(t *testing.T) {
		m, err := async.Await(col.FindOneByID(ctx, "m1"))
		require.NoError(t, err)
		assert.Equal(t, "Alien", m.Title)
		assert.Equal(t, 8.5, m.Rating.Imdb)
		assert.True(t, date(1979).Equal(m.Released))
		assert.Equal(t, int64(2024), m.CreatedOn)
		f, _ := m.Embedding.Float32OK()
		assert.Equal(t, []float32{1, 0, 0}, f)

		upd := movies[0]
		upd.Plot = "Updated plot."
		_, err = async.Await(col.Save(ctx, upd))
		require.NoError(t, err)
		m, err = async.Await(col.FindOneByID(ctx, "m1"))
		require.NoError(t, err)
		assert.Equal(t, "Updated plot.", m.Plot)
		assert.Equal(t, int64(2024), m.CreatedOn, "createdOn survives the update")
		assert.Equal(t, int64(2024), m.UpdatedOn)
		upd.Plot = movies[0].Plot
		_, _ = async.Await(col.Save(ctx, upd))

		_, err = async.Await(col.FindOneByID(ctx, "missing"))
		assert.ErrorIs(t, err, ErrNoDocuments)
	})

	t.Run("FindSql", func(t *testing.T) {
		res, err := async.Await(col.FindSql(ctx, "WHERE (doc->>'year')::int >= $1 ORDER BY doc->'year' DESC LIMIT 2", 1986))
		require.NoError(t, err)
		require.Len(t, res, 2)
		assert.Equal(t, "m4", res[0].ID)
		assert.Equal(t, "m3", res[1].ID)
		assert.Equal(t, 7.2, res[0].Rating.Imdb)

		res, err = async.Await(col.FindSql(ctx, ""))
		require.NoError(t, err)
		assert.Len(t, res, 4)

		// Reachable from the portable interface by type assertion.
		var portable OdmCollectionInterface[pgMovie] = col
		_, ok := portable.(PgCollection[pgMovie])
		assert.True(t, ok)
	})

	t.Run("unsupported features wrap errors.ErrUnsupported", func(t *testing.T) {
		_, err := async.Await(col.Find(ctx, bson.M{"loc": bson.M{"$near": bson.A{0, 0}}}, nil, 0, 0))
		assert.ErrorIs(t, err, errors.ErrUnsupported)
		assert.ErrorContains(t, err, "$near")

		_, err = async.Await(col.Aggregate(ctx, mongo.Pipeline{{{Key: "$group", Value: bson.M{}}}}))
		assert.ErrorIs(t, err, errors.ErrUnsupported)

		err = EnsurePgIndexes[pgTTLModel](ctx, client, tenant)
		assert.ErrorIs(t, err, errors.ErrUnsupported)
		assert.ErrorContains(t, err, "ExpireAfterSeconds")
	})

	t.Run("Find filters", func(t *testing.T) {
		ids := func(filter bson.M, sort bson.D, limit, skip int64) []string {
			res, err := async.Await(col.Find(ctx, filter, sort, limit, skip))
			require.NoError(t, err, "%v", filter)
			out := []string{}
			for _, m := range res {
				out = append(out, m.ID)
			}
			return out
		}
		byYear := bson.D{{Key: "year", Value: 1}}

		assert.Equal(t, []string{"m1", "m2", "m3", "m4"}, ids(nil, byYear, 0, 0))
		assert.Equal(t, []string{"m2", "m3"}, ids(nil, byYear, 2, 1))
		assert.Equal(t, []string{"m4", "m3", "m2", "m1"}, ids(bson.M{}, bson.D{{Key: "year", Value: -1}}, 0, 0))
		assert.Equal(t, []string{"m2"}, ids(bson.M{"year": 1986}, nil, 0, 0))
		assert.Equal(t, []string{"m1", "m2"}, ids(bson.M{"genres": "sci-fi"}, byYear, 0, 0))
		assert.Equal(t, []string{"m3", "m4"}, ids(bson.M{"year": bson.M{"$gt": 1986, "$lte": 1998}}, byYear, 0, 0))
		assert.Equal(t, []string{"m1", "m2", "m3"}, ids(bson.M{"rating.imdb": bson.M{"$gte": 8.0}}, byYear, 0, 0))
		assert.Equal(t, []string{"m3", "m4"}, ids(bson.M{"released": bson.M{"$gte": date(1990)}}, byYear, 0, 0))
		assert.Equal(t, []string{"m2", "m4"}, ids(bson.M{"genres": bson.M{"$in": bson.A{"action"}}}, byYear, 0, 0))
		assert.Equal(t, []string{"m3"}, ids(bson.M{"genres": bson.M{"$nin": bson.A{"action", "sci-fi"}}}, byYear, 0, 0))
		assert.Equal(t, []string{"m1", "m3"}, ids(bson.M{"_id": bson.M{"$in": bson.A{"m1", "m3"}}}, byYear, 0, 0))
		assert.Equal(t, []string{"m1", "m3", "m4"}, ids(bson.M{"_id": bson.M{"$ne": "m2"}}, byYear, 0, 0))
		assert.Equal(t, []string{"m1", "m2"}, ids(bson.M{"title": bson.M{"$regex": "^ali", "$options": "i"}}, byYear, 0, 0))
		assert.Equal(t, []string{"m3"}, ids(bson.M{"genres": bson.M{"$size": 1}}, byYear, 0, 0))
		assert.Equal(t, []string{"m4"}, ids(bson.M{"genres": bson.M{"$all": bson.A{"crime", "action"}}}, byYear, 0, 0))
		assert.Equal(t, []string{"m3", "m4"}, ids(bson.M{"genres": bson.M{"$elemMatch": bson.M{"$eq": "crime"}}}, byYear, 0, 0))
		assert.Equal(t, []string{"m1", "m4"}, ids(bson.M{"$or": bson.A{bson.M{"year": 1979}, bson.M{"title": "Ronin"}}}, byYear, 0, 0))
		assert.Equal(t, []string{"m2", "m3"}, ids(bson.M{"$nor": bson.A{bson.M{"year": 1979}, bson.M{"title": "Ronin"}}}, byYear, 0, 0))
		assert.Equal(t, []string{}, ids(bson.M{"missing": bson.M{"$exists": true}}, nil, 0, 0))
		assert.Equal(t, []string{"m1", "m2", "m3", "m4"}, ids(bson.M{"missing": nil}, byYear, 0, 0))
	})

	t.Run("Count, Exists, Distinct", func(t *testing.T) {
		n, err := async.Await(col.Count(ctx, bson.M{"genres": "crime"}))
		require.NoError(t, err)
		assert.Equal(t, int64(2), n)

		ok, err := async.Await(col.Exists(ctx, "m2"))
		require.NoError(t, err)
		assert.True(t, ok)

		var genres []string
		require.NoError(t, col.DistinctInto(ctx, "genres", nil, &genres))
		assert.ElementsMatch(t, []string{"action", "crime", "horror", "sci-fi"}, genres)

		var years []int
		require.NoError(t, col.DistinctInto(ctx, "year", bson.D{{Key: "genres", Value: "crime"}}, &years))
		assert.ElementsMatch(t, []int{1995, 1998}, years)
	})

	t.Run("Aggregate", func(t *testing.T) {
		res, err := async.Await(col.Aggregate(ctx, mongo.Pipeline{
			{{Key: "$match", Value: bson.M{"genres": "sci-fi"}}},
			{{Key: "$sort", Value: bson.D{{Key: "year", Value: -1}}}},
			{{Key: "$limit", Value: 1}},
		}))
		require.NoError(t, err)
		require.Len(t, res, 1)
		assert.Equal(t, "m2", res[0].ID)

		// $match after $limit must apply to the limited set.
		res, err = async.Await(col.Aggregate(ctx, mongo.Pipeline{
			{{Key: "$sort", Value: bson.D{{Key: "year", Value: 1}}}},
			{{Key: "$limit", Value: 2}},
			{{Key: "$match", Value: bson.M{"genres": "action"}}},
		}))
		require.NoError(t, err)
		require.Len(t, res, 1)
		assert.Equal(t, "m2", res[0].ID)
	})

	t.Run("VectorSearch", func(t *testing.T) {
		hits, err := async.Await(col.VectorSearch(ctx, []float32{1, 0.05, 0}, VectorSearchParams{
			IndexName: "embeddingIndex", Path: "embedding", K: 2, NumCandidates: 50,
		}))
		require.NoError(t, err)
		require.Len(t, hits, 2)
		assert.Equal(t, "m1", hits[0].Doc.ID)
		assert.Equal(t, "m2", hits[1].Doc.ID)
		assert.Greater(t, hits[0].Score, hits[1].Score)
		assert.LessOrEqual(t, hits[0].Score, 1.0)

		hits, err = async.Await(col.VectorSearch(ctx, []float32{1, 0, 0}, VectorSearchParams{
			IndexName: "embeddingIndex", Path: "embedding", K: 2, Filter: bson.M{"year": bson.M{"$gt": 1990}},
		}))
		require.NoError(t, err)
		require.Len(t, hits, 2)
		assert.ElementsMatch(t, []string{"m3", "m4"}, []string{hits[0].Doc.ID, hits[1].Doc.ID})

		_, err = async.Await(col.VectorSearch(ctx, []float32{1, 0}, VectorSearchParams{IndexName: "embeddingIndex", Path: "embedding", K: 1}))
		assert.Error(t, err, "dimension mismatch")
	})

	t.Run("TermSearch", func(t *testing.T) {
		hits, err := async.Await(col.TermSearch(ctx, "crew robbers", TermSearchParams{
			IndexName: "plotIndex", Path: []string{"title", "plot"}, Limit: 10,
		}))
		require.NoError(t, err)
		require.Len(t, hits, 2, "any word matches")
		assert.Equal(t, "m3", hits[0].Doc.ID, "matching both words ranks first")

		hits, err = async.Await(col.TermSearch(ctx, "crew", TermSearchParams{
			IndexName: "plotIndex", Path: []string{"title", "plot"}, Limit: 10, Filter: bson.M{"year": 1979},
		}))
		require.NoError(t, err)
		require.Len(t, hits, 1)
		assert.Equal(t, "m1", hits[0].Doc.ID)
	})

	t.Run("unique index", func(t *testing.T) {
		_, err := async.Await(col.Save(ctx, pgMovie{ID: "dup", Title: "Heat", Embedding: bson.NewVector([]float32{1, 1, 1})}))
		assert.Error(t, err)
	})

	t.Run("Delete", func(t *testing.T) {
		_, err := async.Await(col.DeleteByID(ctx, "m4"))
		require.NoError(t, err)
		_, err = async.Await(col.DeleteOne(ctx, bson.M{"genres": "sci-fi"}))
		require.NoError(t, err)
		n, err := async.Await(col.Count(ctx, nil))
		require.NoError(t, err)
		assert.Equal(t, int64(2), n, "DeleteOne removes a single match")
	})
}
