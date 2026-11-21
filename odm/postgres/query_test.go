package postgres

import (
	"testing"

	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unit tests: no database needed.

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
