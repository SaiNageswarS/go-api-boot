// Integration tests. They need ParadeDB (pg_search; the image ships pgvector):
//
//	docker run -d -e POSTGRES_PASSWORD=test -p 55433:5432 paradedb/paradedb:latest
//	POSTGRES_URI=postgres://postgres:test@localhost:55433/postgres go test ./odm/postgres
//
// They are skipped when POSTGRES_URI is not set.

package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type person struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// stamps is embedded (and unexported) to check that its fields flatten.
type stamps struct {
	CreatedOn int64 `db:"created_on"`
	UpdatedOn int64 `db:"updated_on"`
}

type movie struct {
	ID        string    `db:"id"`
	Title     string    `db:"title"`
	Plot      string    `db:"plot"`
	Year      int       `db:"year"`
	Genres    []string  `db:"genres"`
	Cast      []person  `db:"actors"` // jsonb
	Rating    float64   `db:"rating"`
	Released  time.Time `db:"released"`
	Note      *string   `db:"note"`
	Embedding []float32 `db:"embedding"`
	stamps
}

func (m movie) Id() string             { return m.ID }
func (m movie) CollectionName() string { return "movies" }

func (m movie) Indexes() []odm.IndexSpec {
	return []odm.IndexSpec{
		{Keys: []odm.IndexKey{odm.Key("title")}, Unique: true},
		{Name: "byYear", Keys: []odm.IndexKey{odm.DescKey("year"), odm.Key("rating")}},
	}
}

func (m movie) TermSearchIndexes() []odm.TermSearchIndexSpec {
	return []odm.TermSearchIndexSpec{{Name: "plotIndex", Paths: []string{"title", "plot"}}}
}

func (m movie) VectorIndexes() []odm.VectorIndexSpec {
	return []odm.VectorIndexSpec{{Name: "embeddingIndex", Path: "embedding", Dimensions: 3, Similarity: odm.Cosine}}
}

func date(year int) time.Time { return time.Date(year, 6, 1, 0, 0, 0, 0, time.UTC) }

func fixtures() []movie {
	return []movie{
		{ID: "m1", Title: "Alien", Plot: "A crew aboard a spaceship meets a deadly alien creature.", Year: 1979,
			Genres: []string{"horror", "sci-fi"}, Cast: []person{{"p1", "Sigourney"}, {"p2", "Tom"}}, Rating: 8.5,
			Released: date(1965), Embedding: []float32{1, 0, 0}},
		{ID: "m2", Title: "Aliens", Plot: "Marines fight aliens on a distant colony.", Year: 1986,
			Genres: []string{"action", "sci-fi"}, Cast: []person{{"p1", "Sigourney"}}, Rating: 8.4,
			Released: date(1985), Embedding: []float32{0.9, 0.1, 0}},
		{ID: "m3", Title: "Heat", Plot: "A detective hunts a crew of bank robbers in Los Angeles.", Year: 1995,
			Genres: []string{"crime"}, Cast: []person{{"p3", "Al"}}, Rating: 8.3,
			Released: date(1995), Embedding: []float32{0, 1, 0}},
		{ID: "m4", Title: "Ronin", Plot: "Mercenaries chase a mysterious case across France.", Year: 1998,
			Genres: []string{"action", "crime"}, Cast: []person{{"p4", "Robert"}}, Rating: 7.2,
			Released: date(1998), Embedding: []float32{0, 0.2, 1}},
	}
}

type clock struct{ now atomic.Int64 }

func (c *clock) Now() int64     { return c.now.Load() }
func (c *clock) Set(unix int64) { c.now.Store(unix) }

func openTestDB(t *testing.T, opts ...Option) *DB {
	t.Helper()
	if os.Getenv("POSTGRES_URI") == "" {
		t.Skip("POSTGRES_URI not set")
	}
	db, err := OpenFromEnv(t.Context(), opts...)
	require.NoError(t, err)
	t.Cleanup(db.Close)
	return db
}

// testTenant returns a unique tenant (schema) name.
func testTenant(name string) string {
	return fmt.Sprintf("%s_%s", name, strconv.FormatInt(time.Now().UnixNano(), 36))
}

// seed creates the movies table in a new tenant and saves the fixtures.
func seed(t *testing.T, db *DB) (Table[movie], string) {
	t.Helper()
	ctx := t.Context()
	tenant := testTenant(strings.ToLower(strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))[:min(20, len(t.Name()))])
	t.Cleanup(func() { dropSchema(db, tenant) })
	require.NoError(t, EnsureCollection[movie](ctx, db, tenant))
	require.NoError(t, EnsureCollection[movie](ctx, db, tenant), "EnsureCollection is idempotent")
	movies := TableOf[movie](db, tenant)
	for _, m := range fixtures() {
		_, err := async.Await(movies.Save(ctx, m))
		require.NoError(t, err)
	}
	return movies, tenant
}

func dropSchema(db *DB, tenant string) {
	_, _ = db.pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+pgx.Identifier{tenant}.Sanitize()+" CASCADE")
}

func ids(t *testing.T, q Table[movie]) []string {
	t.Helper()
	rows, err := async.Await(q.Find(t.Context()))
	require.NoError(t, err)
	out := []string{}
	for _, m := range rows {
		out = append(out, m.ID)
	}
	return out
}

func TestQueries(t *testing.T) {
	db := openTestDB(t)
	movies, _ := seed(t, db)
	byYear := movies.OrderBy("year")

	cases := []struct {
		name string
		q    Table[movie]
		want []string
	}{
		{"all", byYear, []string{"m1", "m2", "m3", "m4"}},
		{"comparison", byYear.Where("year > ?", 1986), []string{"m3", "m4"}},
		{"conditions AND together", byYear.Where("year > ?", 1980).Where("rating >= ?", 8.3), []string{"m2", "m3"}},
		{"or inside one condition", byYear.Where("year = ? OR title = ?", 1979, "Ronin"), []string{"m1", "m4"}},
		{"array contains", byYear.Where("? = ANY(genres)", "sci-fi"), []string{"m1", "m2"}},
		{"array overlap", byYear.Where("genres && ?", []string{"horror", "crime"}), []string{"m1", "m3", "m4"}},
		{"list", byYear.Where("id = ANY(?)", []string{"m1", "m3"}), []string{"m1", "m3"}},
		{"timestamps", byYear.Where("released < ?", date(1990)), []string{"m1", "m2"}},
		{"jsonb", byYear.Where("actors @> ?::jsonb", `[{"name":"Sigourney"}]`), []string{"m1", "m2"}},
		{"jsonb ?? operator", byYear.Where("actors->0 ?? 'name' AND year < ?", 1980), []string{"m1"}},
		{"null", byYear.Where("note IS NULL"), []string{"m1", "m2", "m3", "m4"}},
		{"descending", movies.OrderBy("year DESC"), []string{"m4", "m3", "m2", "m1"}},
		{"limit and offset", byYear.Limit(2).Offset(1), []string{"m2", "m3"}},
		{"limit 0", byYear.Limit(0), []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ids(t, tc.q))
		})
	}

	t.Run("round trip", func(t *testing.T) {
		m, err := async.Await(movies.FindByID(t.Context(), "m1"))
		require.NoError(t, err)
		want := fixtures()[0]
		assert.True(t, want.Released.Equal(m.Released))
		m.Released, want.Released = time.Time{}, time.Time{}
		m.stamps = stamps{}
		assert.Equal(t, want, *m)
	})

	t.Run("First, Count, Exists", func(t *testing.T) {
		ctx := t.Context()
		first, err := async.Await(movies.Where("? = ANY(genres)", "crime").OrderBy("rating DESC").First(ctx))
		require.NoError(t, err)
		assert.Equal(t, "m3", first.ID)

		_, err = async.Await(movies.Where("year = ?", 1900).First(ctx))
		assert.ErrorIs(t, err, odm.ErrNotFound)
		_, err = async.Await(movies.FindByID(ctx, "nope"))
		assert.ErrorIs(t, err, odm.ErrNotFound)

		n, err := async.Await(movies.Where("? = ANY(genres)", "crime").Count(ctx))
		require.NoError(t, err)
		assert.Equal(t, int64(2), n)
		n, err = async.Await(movies.Limit(3).Count(ctx))
		require.NoError(t, err)
		assert.Equal(t, int64(3), n, "within the limit")

		ok, err := async.Await(movies.Where("title = ?", "Heat").Exists(ctx))
		require.NoError(t, err)
		assert.True(t, ok)
		ok, err = async.Await(movies.Where("title = ?", "Nope").Exists(ctx))
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("SQL errors name the query", func(t *testing.T) {
		_, err := async.Await(movies.Where("no_such_column = ?", 1).Find(t.Context()))
		assert.ErrorContains(t, err, "no_such_column")
		assert.ErrorContains(t, err, `in SELECT`)
	})
}

func TestSave(t *testing.T) {
	clk := &clock{}
	db := openTestDB(t, WithTimer(clk))
	ctx := t.Context()
	clk.Set(100)
	movies, _ := seed(t, db)

	m, err := async.Await(movies.FindByID(ctx, "m1"))
	require.NoError(t, err)
	assert.Equal(t, stamps{CreatedOn: 100, UpdatedOn: 100}, m.stamps)

	note := "classic"
	changed := fixtures()[0]
	changed.Note, changed.Rating, changed.Cast, changed.Embedding = &note, 9, nil, nil
	changed.CreatedOn = 1 // ignored: Save owns the stamps
	clk.Set(200)
	_, err = async.Await(movies.Save(ctx, changed))
	require.NoError(t, err)

	m, err = async.Await(movies.FindByID(ctx, "m1"))
	require.NoError(t, err)
	assert.Equal(t, "classic", *m.Note)
	assert.Equal(t, 9.0, m.Rating)
	assert.Nil(t, m.Cast, "a nil jsonb value round-trips")
	assert.Nil(t, m.Embedding, "no embedding is NULL")
	assert.Equal(t, stamps{CreatedOn: 100, UpdatedOn: 200}, m.stamps, "created_on is kept")

	t.Run("errors", func(t *testing.T) {
		_, err := async.Await(movies.Save(ctx, movie{Title: "No id"}))
		assert.ErrorContains(t, err, "must not be empty")

		dup := fixtures()[2]
		dup.ID = "dup"
		_, err = async.Await(movies.Save(ctx, dup))
		assert.ErrorIs(t, err, odm.ErrDuplicateKey)

		_, err = async.Await(movies.Save(ctx, movie{ID: "x", Title: "X", Embedding: []float32{1, 2}}))
		assert.ErrorContains(t, err, "expected 3 dimensions")
	})

	t.Run("Delete", func(t *testing.T) {
		n, err := async.Await(movies.Where("? = ANY(genres)", "sci-fi").Delete(ctx))
		require.NoError(t, err)
		assert.Equal(t, int64(2), n)
		_, err = async.Await(movies.DeleteByID(ctx, "m4"))
		require.NoError(t, err)
		_, err = async.Await(movies.DeleteByID(ctx, "m4"))
		require.NoError(t, err, "deleting a missing row is fine")
		assert.Equal(t, []string{"m3"}, ids(t, movies))
	})
}

func TestMissingTable(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	missing := TableOf[movie](db, testTenant("missing"))

	_, err := async.Await(missing.FindByID(ctx, "m1"))
	assert.ErrorIs(t, err, odm.ErrNotFound)
	assert.Equal(t, []string{}, ids(t, missing))
	n, err := async.Await(missing.Count(ctx))
	require.NoError(t, err)
	assert.Zero(t, n)
	ok, err := async.Await(missing.Exists(ctx))
	require.NoError(t, err)
	assert.False(t, ok)

	_, err = async.Await(missing.Save(ctx, fixtures()[0]))
	assert.ErrorContains(t, err, "EnsureCollection")
}

func TestVectorSearch(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	movies, _ := seed(t, db)
	_, err := async.Await(movies.Save(ctx, movie{ID: "none", Title: "No embedding", Year: 2001}))
	require.NoError(t, err)

	hits, err := async.Await(movies.VectorSearch(ctx, "embeddingIndex", []float32{1, 0.05, 0}, 2))
	require.NoError(t, err)
	require.Len(t, hits, 2)
	assert.Equal(t, "m1", hits[0].Doc.ID)
	assert.Equal(t, "m2", hits[1].Doc.ID)
	assert.Greater(t, hits[0].Score, hits[1].Score)
	assert.LessOrEqual(t, hits[0].Score, 1.0)
	assert.Equal(t, []float32{1, 0, 0}, hits[0].Doc.Embedding)

	hits, err = async.Await(movies.Where("year > ?", 1990).VectorSearch(ctx, "embeddingIndex", []float32{1, 0, 0}, 2))
	require.NoError(t, err)
	require.Len(t, hits, 2, "filtering doesn't cut results short")
	assert.ElementsMatch(t, []string{"m3", "m4"}, []string{hits[0].Doc.ID, hits[1].Doc.ID})

	hits, err = async.Await(movies.VectorSearch(ctx, "embeddingIndex", []float32{1, 0, 0}, 100))
	require.NoError(t, err)
	assert.Len(t, hits, 4, "rows without an embedding are skipped")
}

func TestTermSearch(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	movies, _ := seed(t, db)

	hits, err := async.Await(movies.TermSearch(ctx, "plotIndex", "crew robbers", 10))
	require.NoError(t, err)
	require.Len(t, hits, 2, "any word matches")
	assert.Equal(t, "m3", hits[0].Doc.ID, "matching both words ranks first")
	assert.Greater(t, hits[0].Score, hits[1].Score)

	hits, err = async.Await(movies.Where("year = ?", 1979).TermSearch(ctx, "plotIndex", "crew", 10))
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, "m1", hits[0].Doc.ID)

	hits, err = async.Await(movies.TermSearch(ctx, "plotIndex", "mercenary", 10))
	require.NoError(t, err)
	require.Len(t, hits, 1, "stemming")
	assert.Equal(t, "m4", hits[0].Doc.ID)

	_, err = async.Await(movies.TermSearch(ctx, "nope", "crew", 1))
	assert.ErrorContains(t, err, "no term search index")
}

/* ── EnsureCollection ── */

// movieV2 changes the schema: a new column, different indexes.
type movieV2 struct {
	movie
	Studio string `db:"studio"`
}

func (movieV2) Indexes() []odm.IndexSpec {
	// title is no longer unique; byYear is gone; a new index on studio.
	return []odm.IndexSpec{{Keys: []odm.IndexKey{odm.Key("title")}}, {Keys: []odm.IndexKey{odm.Key("studio")}}}
}
func (movieV2) TermSearchIndexes() []odm.TermSearchIndexSpec {
	return []odm.TermSearchIndexSpec{{Name: "plotIndex", Paths: []string{"plot", "studio"}}}
}
func (movieV2) VectorIndexes() []odm.VectorIndexSpec {
	return []odm.VectorIndexSpec{{Name: "embeddingIndex", Path: "embedding", Dimensions: 3, Similarity: odm.Euclidean}}
}

func managedIndexes(t *testing.T, db *DB, tenant string) []string {
	t.Helper()
	rows, err := db.pool.Query(t.Context(),
		`SELECT pg_get_indexdef(c.oid) FROM pg_index x JOIN pg_class c ON c.oid = x.indexrelid
		  WHERE x.indrelid = $1::regclass AND obj_description(c.oid, 'pg_class') = 'odm' ORDER BY 1`,
		pgx.Identifier{tenant, "movies"}.Sanitize())
	require.NoError(t, err)
	defs, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	return defs
}

func TestEnsureCollectionMigrates(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	_, tenant := seed(t, db)
	before := managedIndexes(t, db, tenant)
	require.Len(t, before, 4, "title, byYear, embedding, search")

	require.NoError(t, EnsureCollection[movieV2](ctx, db, tenant))
	after := strings.Join(managedIndexes(t, db, tenant), "\n")
	assert.NotContains(t, after, "UNIQUE")
	assert.Contains(t, after, "vector_l2_ops")
	assert.Contains(t, after, "studio")

	// Existing rows read with the new column's default.
	v2 := TableOf[movieV2](db, tenant)
	m, err := async.Await(v2.FindByID(ctx, "m1"))
	require.NoError(t, err)
	assert.Equal(t, "", m.Studio)

	// The new column is searchable, and duplicate titles are allowed now.
	dup := movieV2{movie: fixtures()[2], Studio: "Warner"}
	dup.ID = "dup"
	_, err = async.Await(v2.Save(ctx, dup))
	require.NoError(t, err)
	hits, err := async.Await(v2.TermSearch(ctx, "plotIndex", "warner", 10))
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, "dup", hits[0].Doc.ID)

	// Back to the original model: the same index names as before.
	_, err = async.Await(v2.DeleteByID(ctx, "dup"))
	require.NoError(t, err)
	require.NoError(t, EnsureCollection[movie](ctx, db, tenant))
	assert.Equal(t, before, managedIndexes(t, db, tenant))
}

// movieYearText changes the type of an existing column.
type movieYearText struct {
	ID   string `db:"id"`
	Year string `db:"year"`
}

func (m movieYearText) Id() string           { return m.ID }
func (movieYearText) CollectionName() string { return "movies" }

func TestEnsureCollectionRejectsTypeChanges(t *testing.T) {
	db := openTestDB(t)
	_, tenant := seed(t, db)
	err := EnsureCollection[movieYearText](t.Context(), db, tenant)
	assert.ErrorContains(t, err, "column movies.year is bigint but the model needs text")
}

type ttlModel struct {
	ID        string `db:"id"`
	CreatedOn int64  `db:"created_on"`
}

func (m ttlModel) Id() string           { return m.ID }
func (ttlModel) CollectionName() string { return "sessions" }
func (ttlModel) Indexes() []odm.IndexSpec {
	return []odm.IndexSpec{{Keys: []odm.IndexKey{odm.Key("created_on")}, TTL: time.Hour}}
}

func TestEnsureCollectionRejects(t *testing.T) {
	db := openTestDB(t)
	err := EnsureCollection[ttlModel](t.Context(), db, testTenant("ttl"))
	assert.ErrorIs(t, err, errors.ErrUnsupported)
}
