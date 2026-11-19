package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-api-boot/odm/internal/conformance"
	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Unit tests for SQL generation. Behaviour is covered against a real database
// by the conformance suite (collection_test.go).

func TestWhere_RowIDOnlyAtTopLevel(t *testing.T) {
	q := &pgQuery{}
	sql, err := q.where(odm.M{"_id": "a"})
	require.NoError(t, err)
	assert.Equal(t, "((id = $1))", sql)

	q = &pgQuery{}
	sql, err = q.where(odm.M{"kids": odm.M{"$elemMatch": odm.M{"_id": "a"}}})
	require.NoError(t, err)
	assert.NotContains(t, sql, "id = ")
	assert.Contains(t, sql, "doc @> ")
}

func TestWhere_Regex(t *testing.T) {
	for options, prefix := range map[string]string{"": "(?p)", "m": "(?n)", "s": "", "ms": "(?w)", "i": "(?ip)", "ix": "(?ixp)"} {
		q := &pgQuery{}
		_, err := q.where(odm.M{"name": odm.M{"$regex": "^a", "$options": options}})
		require.NoError(t, err)
		assert.Equal(t, prefix+"^a", q.args[0], "options %q", options)
	}
}

func TestWhere_ContainmentVariants(t *testing.T) {
	variants, err := containmentVariants([]string{"cast", "name"}, "Al")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		`{"cast":{"name":"Al"}}`, `{"cast":[{"name":"Al"}]}`,
		`{"cast":{"name":["Al"]}}`, `{"cast":[{"name":["Al"]}]}`,
	}, variants)

	variants, err = containmentVariants([]string{"meta"}, odm.M{"a": 1})
	require.NoError(t, err)
	assert.Equal(t, []string{`{"meta":{"a":1}}`}, variants, "documents don't match inside arrays")
}

func TestWhere_Unsupported(t *testing.T) {
	for _, f := range []odm.M{
		{"$text": odm.M{"$search": "x"}},
		{"x": odm.M{"$near": 1}},
		{"x": odm.M{"$regex": "a", "$options": "u"}},
	} {
		_, err := (&pgQuery{}).where(f)
		assert.ErrorIs(t, err, errors.ErrUnsupported, "%v", f)
	}
	_, err := (&pgQuery{}).where(odm.M{"$or": odm.A{}})
	assert.Error(t, err)
}

func TestOrderBy(t *testing.T) {
	assert.Equal(t, ` ORDER BY (doc #> '{"year"}') DESC NULLS LAST, id ASC NULLS FIRST`,
		orderBy([]odm.SortField{odm.Desc("year"), odm.Asc("_id")}))
	assert.Equal(t, "", orderBy(nil))
}

func TestPathLiteralEscapes(t *testing.T) {
	assert.Equal(t, `(doc #> '{"we''ird\"key"}')`, jsonbExpr([]string{`we'ird"key`}))
}

func TestJSONRoundTrip(t *testing.T) {
	type doc struct {
		ID  string      `bson:"_id"`
		Vec bson.Vector `bson:"vec"`
		F32 []float32   `bson:"f32"`
		At  time.Time   `bson:"at"`
	}
	in := doc{ID: "a", Vec: bson.NewVector([]float32{0.5, -1}), F32: []float32{1, 2.5}, At: time.Date(1965, 1, 2, 3, 4, 5, 0, time.UTC)}

	js, err := bsonToJSON(in)
	require.NoError(t, err)
	assert.JSONEq(t, `{"_id":"a","vec":[0.5,-1.0],"f32":[1.0,2.5],"at":{"$date":"1965-01-02T03:04:05.000Z"}}`, js,
		"vectors become arrays; every date uses one sortable format")

	out, err := jsonToModel[doc]([]byte(js))
	require.NoError(t, err)
	assert.Equal(t, in.F32, out.F32)
	assert.True(t, in.At.Equal(out.At))
	f, ok := out.Vec.Float32OK()
	assert.True(t, ok)
	assert.Equal(t, []float32{0.5, -1}, f)
}

/* ── a database without pg_search ── */

type noPgSearchPool struct{ Pool }

type boolRow bool

func (r boolRow) Scan(dest ...any) error { *(dest[0].(*bool)) = bool(r); return nil }

func (noPgSearchPool) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New(`extension "pg_search" is not available`)
}
func (noPgSearchPool) QueryRow(context.Context, string, ...any) pgx.Row { return boolRow(false) }

type searchOnly struct{ conformance.Movie }

func (searchOnly) Indexes() []odm.IndexSpec             { return nil }
func (searchOnly) VectorIndexes() []odm.VectorIndexSpec { return nil }

func TestPgSearchRequired(t *testing.T) {
	db := New(noPgSearchPool{})
	err := EnsureCollection[searchOnly](t.Context(), db, "tenant")
	assert.ErrorIs(t, err, ErrPgSearchNotInstalled)

	_, err = async.Await(CollectionOf[searchOnly](db, "tenant").TermSearch(t.Context(), "crew",
		odm.TermSearchParams{IndexName: "plotIndex", Limit: 5}))
	assert.ErrorIs(t, err, ErrPgSearchNotInstalled)
}
