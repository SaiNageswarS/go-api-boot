package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unit tests: no database needed.

func TestPlaceholders(t *testing.T) {
	cases := []struct{ in, out string }{
		{"score > ?", "score > $3"},
		{"a = ? AND b = ?", "a = $3 AND b = $4"},
		{"meta ?? 'k' AND x = ?", "meta ? 'k' AND x = $3"},
		{"name = 'what?' AND x = ?", "name = 'what?' AND x = $3"},
		{`"odd?col" = ?`, `"odd?col" = $3`},
		{"it''s = 'it''s?' AND x = ?", "it''s = 'it''s?' AND x = $3"},
		{"x = ? -- why?\nAND y = ?", "x = $3 -- why?\nAND y = $4"},
		{"x = ? /* ? */", "x = $3 /* ? */"},
	}
	for _, c := range cases {
		assert.Equal(t, c.out, bindPlaceholders(c.in, 2), c.in)
	}
	assert.Equal(t, 2, countPlaceholders("a = ? AND b = ? AND c ?? 'k' AND d = '?'"))
}

type address struct {
	City string `json:"city"`
}

type base struct {
	ID        string `db:"id"`
	CreatedOn int64  `db:"created_on"`
}

type profile struct {
	base
	Name      string
	UserID    string
	Score     float64
	Rank      int32
	Tags      []string
	Address   address
	Previous  *address
	Nickname  *string
	Joined    time.Time
	Avatar    []byte
	Embedding []float32 `db:"embedding"`
	Secret    string    `db:"-"`
	hidden    string
}

func (p profile) Id() string             { return p.ID }
func (p profile) CollectionName() string { return "profiles" }
func (p profile) VectorIndexes() []odm.VectorIndexSpec {
	return []odm.VectorIndexSpec{{Name: "byEmbedding", Path: "embedding", Dimensions: 3}}
}

func TestSchemaOf(t *testing.T) {
	_ = profile{}.hidden
	s, err := schemaOf[profile]()
	require.NoError(t, err)

	got := map[string]string{}
	var order []string
	for _, c := range s.columns {
		got[c.name] = c.definition()
		order = append(order, c.name)
	}
	assert.Equal(t, []string{"id", "created_on", "name", "user_id", "score", "rank", "tags", "address",
		"previous", "nickname", "joined", "avatar", "embedding"}, order, "embedded fields flatten; db:\"-\" and unexported skip")
	assert.Equal(t, map[string]string{
		"id":         `"id" text PRIMARY KEY`,
		"created_on": `"created_on" bigint NOT NULL DEFAULT 0`,
		"name":       `"name" text NOT NULL DEFAULT ''`,
		"user_id":    `"user_id" text NOT NULL DEFAULT ''`,
		"score":      `"score" double precision NOT NULL DEFAULT 0`,
		"rank":       `"rank" integer NOT NULL DEFAULT 0`,
		"tags":       `"tags" text[] NOT NULL DEFAULT '{}'`,
		"address":    `"address" jsonb NOT NULL DEFAULT 'null'`,
		"previous":   `"previous" jsonb NOT NULL DEFAULT 'null'`,
		"nickname":   `"nickname" text`,
		"joined":     `"joined" timestamp with time zone NOT NULL DEFAULT '0001-01-01 00:00:00+00'`,
		"avatar":     `"avatar" bytea NOT NULL DEFAULT '\x'`,
		"embedding":  `"embedding" vector(3)`,
	}, got)
	assert.NotNil(t, s.createdOn)
	assert.Nil(t, s.updatedOn)
}

func TestSnakeCase(t *testing.T) {
	for in, out := range map[string]string{"Name": "name", "UserID": "user_id", "HTTPServer": "http_server", "ReleasedOn": "released_on", "ID": "id"} {
		assert.Equal(t, out, snakeCase(in))
	}
}

type noID struct {
	Key string `db:"key"`
}

func (noID) Id() string             { return "" }
func (noID) CollectionName() string { return "x" }

type twoNames struct {
	ID   string `db:"id"`
	A    string `db:"name"`
	Name string
}

func (twoNames) Id() string             { return "" }
func (twoNames) CollectionName() string { return "x" }

type badVector struct {
	ID string `db:"id"`
	E  []float64
}

func (badVector) Id() string             { return "" }
func (badVector) CollectionName() string { return "x" }
func (badVector) VectorIndexes() []odm.VectorIndexSpec {
	return []odm.VectorIndexSpec{{Name: "v", Path: "e", Dimensions: 3}}
}

func TestSchemaErrors(t *testing.T) {
	_, err := schemaOf[noID]()
	assert.ErrorContains(t, err, `"id" column`)
	_, err = schemaOf[twoNames]()
	assert.ErrorContains(t, err, `two fields to column "name"`)
	_, err = schemaOf[badVector]()
	assert.ErrorContains(t, err, "needs a []float32 field")
}

func TestBuilderIsImmutable(t *testing.T) {
	base := TableOf[profile](nil, "tenant").Where("score > ?", 1)
	a := base.Where("name = ?", "a").OrderBy("score DESC").Limit(5)
	b := base.Where("name = ?", "b")

	qa, err := a.build("*")
	require.NoError(t, err)
	qb, err := b.build("*")
	require.NoError(t, err)
	assert.Equal(t, `SELECT * FROM "tenant"."profiles" WHERE (score > $1) AND (name = $2) ORDER BY score DESC LIMIT 5`, qa.sql)
	assert.Equal(t, []any{1, "a"}, qa.args)
	assert.Equal(t, `SELECT * FROM "tenant"."profiles" WHERE (score > $1) AND (name = $2)`, qb.sql)
	assert.Equal(t, []any{1, "b"}, qb.args)
}

func TestBuilderErrors(t *testing.T) {
	tbl := TableOf[profile](nil, "tenant")
	_, err := tbl.Where("a = ? AND b = ?", 1).build("*")
	assert.ErrorContains(t, err, "2 placeholders but 1 arguments")
	_, err = tbl.Limit(-1).build("*")
	assert.Error(t, err)
	_, err = tbl.Offset(-1).build("*")
	assert.Error(t, err)

	_, err = async.Await(tbl.Delete(t.Context()))
	assert.ErrorContains(t, err, "needs a Where")
	_, err = async.Await(tbl.Where("true").Limit(1).Delete(t.Context()))
	assert.ErrorContains(t, err, "doesn't take")
	_, err = async.Await(tbl.Limit(3).VectorSearch(t.Context(), "byEmbedding", []float32{1, 2, 3}, 3))
	assert.ErrorContains(t, err, "drop OrderBy, Limit and Offset")
	_, err = async.Await(tbl.VectorSearch(t.Context(), "nope", []float32{1, 2, 3}, 3))
	assert.ErrorContains(t, err, `no vector index "nope"`)
	_, err = async.Await(tbl.VectorSearch(t.Context(), "byEmbedding", []float32{1, 2}, 3))
	assert.ErrorContains(t, err, "dimensions")
}

/* ── a database without pg_search ── */

type noPgSearchPool struct{ Pool }

type boolRow bool

func (r boolRow) Scan(dest ...any) error { *(dest[0].(*bool)) = bool(r); return nil }

func (noPgSearchPool) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New(`extension "pg_search" is not available`)
}
func (noPgSearchPool) QueryRow(context.Context, string, ...any) pgx.Row { return boolRow(false) }

type searchOnly struct {
	ID   string `db:"id"`
	Plot string `db:"plot"`
}

func (s searchOnly) Id() string           { return s.ID }
func (searchOnly) CollectionName() string { return "movies" }
func (searchOnly) TermSearchIndexes() []odm.TermSearchIndexSpec {
	return []odm.TermSearchIndexSpec{{Name: "plotIndex", Paths: []string{"plot"}}}
}

func TestPgSearchRequired(t *testing.T) {
	db := New(noPgSearchPool{})
	err := EnsureCollection[searchOnly](t.Context(), db, "tenant")
	assert.ErrorIs(t, err, ErrPgSearchNotInstalled)

	_, err = async.Await(TableOf[searchOnly](db, "tenant").TermSearch(t.Context(), "plotIndex", "crew", 5))
	assert.ErrorIs(t, err, ErrPgSearchNotInstalled)
}
