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
