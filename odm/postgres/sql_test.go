package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestQuoteLiteral(t *testing.T) {
	assert.Equal(t, `'it''s'`, quoteLiteral("it's"))
}

func TestUpsertSQL(t *testing.T) {
	s, err := schemaOf[note]()
	if err != nil {
		t.Fatal(err)
	}
	casts := make([]string, len(s.columns))
	assert.Equal(t, `INSERT INTO "t"."notes" ("id", "text", "created_on", "updated_on") VALUES ($1, $2, $3, $4)`+
		` ON CONFLICT (id) DO UPDATE SET "text" = EXCLUDED."text", "updated_on" = EXCLUDED."updated_on"`,
		upsertSQL(`"t"."notes"`, s.columns, casts))
}

func TestBuilderIsImmutable(t *testing.T) {
	base := TableOf[profile](nil, "tenant").Where("score > ?", 1)
	a := base.Where("name = ?", "a").OrderBy("score DESC").Limit(5)
	b := base.Where("name = ?", "b")

	sql, args := a.selectSQL("*")
	assert.Equal(t, `SELECT * FROM "tenant"."profiles" WHERE (score > $1) AND (name = $2) ORDER BY score DESC LIMIT 5`, sql)
	assert.Equal(t, []any{1, "a"}, args)
	sql, args = b.selectSQL("*")
	assert.Equal(t, `SELECT * FROM "tenant"."profiles" WHERE (score > $1) AND (name = $2)`, sql)
	assert.Equal(t, []any{1, "b"}, args)
}
