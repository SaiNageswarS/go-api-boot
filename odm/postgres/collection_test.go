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
	"strings"
	"testing"
	"time"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-api-boot/odm/internal/conformance"
	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

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

func testTenant(name string) string { return fmt.Sprintf("odm_%s_%d", name, time.Now().UnixNano()) }

func dropSchema(db *DB, tenant string) {
	_, _ = db.pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+pgx.Identifier{tenant}.Sanitize()+" CASCADE")
}

func TestConformance(t *testing.T) {
	clock := &conformance.Clock{}
	db := openTestDB(t, WithTimer(clock))
	conformance.Run(t, conformance.Backend{
		Clock: clock,
		Collection: func(tenant string) odm.Collection[conformance.Movie] {
			return CollectionOf[conformance.Movie](db, tenant)
		},
		Ensure: func(ctx context.Context, tenant string) error {
			return EnsureCollection[conformance.Movie](ctx, db, tenant)
		},
		Drop: func(_ context.Context, tenant string) { dropSchema(db, tenant) },
	}, testTenant("conformance"))
}

func seed(t *testing.T, db *DB, tenant string) Collection[conformance.Movie] {
	t.Helper()
	ctx := t.Context()
	t.Cleanup(func() { dropSchema(db, tenant) })
	require.NoError(t, EnsureCollection[conformance.Movie](ctx, db, tenant))
	col := CollectionOf[conformance.Movie](db, tenant)
	for _, m := range conformance.Fixtures() {
		_, err := async.Await(col.Save(ctx, m))
		require.NoError(t, err)
	}
	return col
}

func TestPostgresSpecifics(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	col := seed(t, db, testTenant("specifics"))

	t.Run("FindSql", func(t *testing.T) {
		res, err := async.Await(col.FindSql(ctx, "WHERE (doc->>'year')::int >= $1 ORDER BY doc->'year' DESC LIMIT 2", 1986))
		require.NoError(t, err)
		require.Len(t, res, 2)
		assert.Equal(t, "m4", res[0].ID)
		assert.Equal(t, "m3", res[1].ID)

		var portable odm.Collection[conformance.Movie] = col
		_, ok := portable.(Collection[conformance.Movie])
		assert.True(t, ok, "reachable by type assertion")
	})

	t.Run("term search stems words", func(t *testing.T) {
		hits, err := async.Await(col.TermSearch(ctx, "mercenary", odm.TermSearchParams{IndexName: "plotIndex", Limit: 10}))
		require.NoError(t, err)
		require.Len(t, hits, 1)
		assert.Equal(t, "m4", hits[0].Doc.ID)
	})

	t.Run("unsupported filters wrap errors.ErrUnsupported", func(t *testing.T) {
		_, err := async.Await(col.Find(ctx, odm.M{"loc": odm.M{"$near": odm.A{0, 0}}}))
		assert.ErrorIs(t, err, errors.ErrUnsupported)
		assert.ErrorContains(t, err, "$near")
	})

	t.Run("writes to a collection that doesn't exist fail clearly", func(t *testing.T) {
		missing := CollectionOf[conformance.Movie](db, testTenant("missing"))
		_, err := async.Await(missing.Save(ctx, conformance.Fixtures()[0]))
		assert.ErrorContains(t, err, "EnsureCollection")
	})
}

/* ── index reconciliation ── */

type movieV2 struct{ conformance.Movie }

func (m movieV2) Indexes() []odm.IndexSpec {
	// title is no longer unique; byYear is gone; a new index on rating.
	return []odm.IndexSpec{{Keys: []odm.IndexKey{odm.Key("title")}}, {Keys: []odm.IndexKey{odm.DescKey("rating")}}}
}
func (m movieV2) TermSearchIndexes() []odm.TermSearchIndexSpec {
	return []odm.TermSearchIndexSpec{{Name: "plotIndex", Paths: []string{"plot", "genres"}}}
}
func (m movieV2) VectorIndexes() []odm.VectorIndexSpec { return nil }

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

func TestEnsureCollectionReconciles(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	tenant := testTenant("reconcile")
	seed(t, db, tenant)

	before := managedIndexes(t, db, tenant)
	require.Len(t, before, 6, "doc, title, byYear, embedding, year filter, search")

	require.NoError(t, EnsureCollection[movieV2](ctx, db, tenant))
	after := strings.Join(managedIndexes(t, db, tenant), "\n")
	assert.NotContains(t, after, "UNIQUE")
	assert.NotContains(t, after, "hnsw")
	assert.Contains(t, after, "rating")
	assert.Contains(t, after, "search_genres")
	assert.NotContains(t, after, "search_title")

	// The new search paths work and old data was re-indexed.
	col := CollectionOf[movieV2](db, tenant)
	hits, err := async.Await(col.TermSearch(ctx, "crime", odm.TermSearchParams{IndexName: "plotIndex", Limit: 10}))
	require.NoError(t, err)
	assert.Len(t, hits, 2)

	// Duplicate titles are allowed now.
	dup := movieV2{conformance.Fixtures()[2]}
	dup.ID = "dup"
	_, err = async.Await(col.Save(ctx, dup))
	assert.NoError(t, err)

	// Back to the original: same index names as before.
	require.NoError(t, EnsureCollection[conformance.Movie](ctx, db, tenant))
	_, _ = async.Await(col.DeleteByID(ctx, "dup"))
	require.NoError(t, EnsureCollection[conformance.Movie](ctx, db, tenant))
	assert.Equal(t, before, managedIndexes(t, db, tenant))
}

type ttlModel struct {
	ID string `bson:"_id"`
}

func (m ttlModel) Id() string             { return m.ID }
func (m ttlModel) CollectionName() string { return "sessions" }
func (m ttlModel) Indexes() []odm.IndexSpec {
	return []odm.IndexSpec{{Keys: []odm.IndexKey{odm.Key("createdOn")}, TTL: time.Hour}}
}

func TestEnsureCollectionRejectsTTL(t *testing.T) {
	db := openTestDB(t)
	tenant := testTenant("ttl")
	t.Cleanup(func() { dropSchema(db, tenant) })
	err := EnsureCollection[ttlModel](t.Context(), db, tenant)
	assert.ErrorIs(t, err, errors.ErrUnsupported)
}

func TestVectorIndexSkipsBadEmbeddings(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	col := seed(t, db, testTenant("vectors"))
	// The partial index skips documents without a 3-number embedding instead
	// of failing their writes.
	_, err := async.Await(col.Save(ctx, conformance.Movie{ID: "x", Title: "X", Embedding: bson.NewVector([]float32{1, 2, 3, 4})}))
	assert.NoError(t, err)
}
