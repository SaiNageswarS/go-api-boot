package odm

import (
	"context"
	"errors"
	"testing"

	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestPgWhere(t *testing.T) {
	cases := []struct {
		name   string
		filter any
		sql    string
		args   []any
	}{
		{"empty", bson.M{}, "TRUE", nil},
		{"nil", nil, "TRUE", nil},
		{"id", bson.M{"_id": "a1"}, "((id = $1))", []any{"a1"}},
		{"scalar eq also matches arrays", bson.M{"tags": "x"},
			"((doc @> $1::jsonb OR doc @> $2::jsonb))", []any{`{"tags":"x"}`, `{"tags":["x"]}`}},
		{"dotted path", bson.M{"a.b": 2},
			"((doc @> $1::jsonb OR doc @> $2::jsonb))", []any{`{"a":{"b":2}}`, `{"a":{"b":[2]}}`}},
		{"null", bson.M{"x": nil}, `(((doc #> '{"x"}') IS NULL OR (doc #> '{"x"}') = 'null'::jsonb))`, nil},
		{"range", bson.M{"year": bson.M{"$gte": 1980}},
			`(((jsonb_typeof((doc #> '{"year"}')) = jsonb_typeof($1::jsonb) AND (doc #> '{"year"}') >= $1::jsonb)))`, []any{"1980"}},
		{"id in", bson.M{"_id": bson.M{"$in": []string{"a", "b"}}}, "(((id = ANY($1::text[]))))", []any{[]string{"a", "b"}}},
		{"empty in", bson.M{"x": bson.M{"$in": bson.A{}}}, "((FALSE))", nil},
		{"exists", bson.M{"x": bson.M{"$exists": false}}, `((((doc #> '{"x"}') IS NULL)))`, nil},
		{"regex", bson.M{"name": bson.M{"$regex": "^ri", "$options": "i"}},
			`(((jsonb_typeof((doc #> '{"name"}')) = 'string' AND (doc #>> '{"name"}') ~ $1)))`, []any{"(?i)^ri"}},
		{"quotes in path are escaped", bson.M{`we'ird"key`: bson.M{"$exists": true}},
			`((((doc #> '{"we''ird\"key"}') IS NOT NULL)))`, nil},
		{"or", bson.M{"$or": bson.A{bson.M{"_id": "a"}, bson.M{"_id": "b"}}}, "((((id = $1)) OR ((id = $2))))", []any{"a", "b"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := &pgQuery{}
			sql, err := q.where(tc.filter)
			assert.NoError(t, err)
			assert.Equal(t, tc.sql, sql)
			assert.Equal(t, tc.args, q.args)
		})
	}
}

func TestPgWhere_Unsupported(t *testing.T) {
	for _, f := range []bson.M{
		{"$text": bson.M{"$search": "x"}},
		{"x": bson.M{"$near": 1}},
		{"x": bson.M{"$regex": "a", "$options": "u"}},
		{"$or": bson.A{}},
	} {
		_, err := (&pgQuery{}).where(f)
		assert.Error(t, err, "%v", f)
	}
}

func TestPgOrderBy(t *testing.T) {
	sql, err := orderBy(bson.D{{Key: "year", Value: -1}, {Key: "_id", Value: 1}})
	assert.NoError(t, err)
	assert.Equal(t, ` ORDER BY (doc #> '{"year"}') DESC NULLS LAST, id ASC NULLS FIRST`, sql)

	sql, err = orderBy(nil)
	assert.NoError(t, err)
	assert.Equal(t, "", sql)

	_, err = orderBy(bson.D{{Key: "x", Value: 2}})
	assert.Error(t, err)
}

func TestPgJSONRoundTrip(t *testing.T) {
	type withVector struct {
		ID  string      `bson:"_id"`
		Vec bson.Vector `bson:"vec"`
		F32 []float32   `bson:"f32"`
	}
	in := withVector{ID: "a", Vec: bson.NewVector([]float32{0.5, -1}), F32: []float32{1, 2.5}}

	js, err := bsonToJSON(in)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"_id":"a","vec":[0.5,-1.0],"f32":[1.0,2.5]}`, js)

	out, err := jsonToModel[withVector]([]byte(js))
	assert.NoError(t, err)
	assert.Equal(t, in.F32, out.F32)
	f, ok := out.Vec.Float32OK()
	assert.True(t, ok)
	assert.Equal(t, []float32{0.5, -1}, f)
}

// noPgSearchClient is a database without the pg_search extension.
type noPgSearchClient struct{ PgClient }

type boolRow bool

func (r boolRow) Scan(dest ...any) error { *(dest[0].(*bool)) = bool(r); return nil }

func (noPgSearchClient) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New(`extension "pg_search" is not available`)
}
func (noPgSearchClient) QueryRow(context.Context, string, ...any) pgx.Row { return boolRow(false) }

func TestPgSearchRequired(t *testing.T) {
	db := &noPgSearchClient{}
	err := ensurePgSearchIndex(t.Context(), db, "tenant", "movies", []string{"plot"})
	assert.ErrorIs(t, err, ErrPgSearchNotInstalled)

	_, err = async.Await(PgCollectionOf[pgMovie](db, "tenant").TermSearch(t.Context(), "crew",
		TermSearchParams{IndexName: "plotIndex", Path: []string{"plot"}, Limit: 5}))
	assert.ErrorIs(t, err, ErrPgSearchNotInstalled)
}
