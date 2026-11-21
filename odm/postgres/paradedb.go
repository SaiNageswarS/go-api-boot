package postgres

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-api-boot/odm/internal/core"
	"github.com/jackc/pgx/v5"
)

// The SQL of term search with ParadeDB's pg_search extension (BM25, like
// Atlas Search). EnsureTable creates one ParadeDB index over the text columns
// of all the model's TermSearchIndexSpecs: ParadeDB allows one per table.

// termSearchSQL selects the limit rows of s best matching query in spec's
// columns, among those matching t's conditions, each followed by its score.
func (t Table[T]) termSearchSQL(s *tableSchema, spec odm.TermSearchIndexSpec, query string, limit int) (string, []any) {
	matches := make([]string, len(spec.Paths))
	for i, p := range spec.Paths {
		matches[i] = pgx.Identifier{p}.Sanitize() + " ||| $1::text"
	}
	where, args := t.whereSQL([]any{query})
	where = andWhere(where, "("+strings.Join(matches, " OR ")+")")
	return "SELECT " + s.selectList() + ", pdb.score(" + idColumn + ")::float8 AS score FROM " + t.qualified() + where +
		" ORDER BY score DESC LIMIT " + strconv.Itoa(limit), args
}

// searchIndexSQL is the definition of the table's ParadeDB index over
// columns.
func searchIndexSQL(table string, columns []string, stemmer string) string {
	tokenizer := "pdb.unicode_words"
	if stemmer != "" {
		tokenizer += "(" + quoteLiteral("stemmer="+stemmer) + ")"
	}
	fields := []string{idColumn}
	for _, c := range columns {
		fields = append(fields, "("+pgx.Identifier{c}.Sanitize()+"::"+tokenizer+")")
	}
	return "ON " + table + " USING paradedb (" + strings.Join(fields, ", ") + ")"
}

// searchColumns checks that the term search paths are text columns of s, and
// returns them without duplicates.
func searchColumns(s *tableSchema, specs []odm.TermSearchIndexSpec) ([]string, error) {
	var columns []string
	for _, spec := range specs {
		if err := core.ValidateTermSearchIndex(spec); err != nil {
			return nil, err
		}
		for _, p := range spec.Paths {
			c, ok := s.byName[p]
			if !ok || c.sqlType != "text" {
				return nil, fmt.Errorf("term search index %s: %q is not a text column", spec.Name, p)
			}
			if !slices.Contains(columns, p) {
				columns = append(columns, p)
			}
		}
	}
	return columns, nil
}
