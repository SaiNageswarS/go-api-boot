package postgres

import (
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// The SQL text Table's methods run: pure functions of the query, no database
// access. Extension-specific SQL is in pgvector.go and paradedb.go.

func (t Table[T]) qualified() string { return pgx.Identifier{t.schema, t.name}.Sanitize() }

// selectSQL is SELECT <list> FROM table WHERE ... ORDER BY ... LIMIT ...
// OFFSET ..., with its arguments.
func (t Table[T]) selectSQL(list string) (string, []any) {
	where, args := t.whereSQL(nil)
	return "SELECT " + list + " FROM " + t.qualified() + where + t.pagingSQL(), args
}

// deleteSQL is DELETE FROM table WHERE ..., with its arguments.
func (t Table[T]) deleteSQL() (string, []any) {
	where, args := t.whereSQL(nil)
	return "DELETE FROM " + t.qualified() + where, args
}

// whereSQL renders the conditions after prefix arguments (numbered from
// len(prefix)+1), returning " WHERE ..." (or "") and all arguments.
func (t Table[T]) whereSQL(prefix []any) (string, []any) {
	args := slices.Clone(prefix)
	if len(t.where) == 0 {
		return "", args
	}
	parts := make([]string, len(t.where))
	for i, cond := range t.where {
		parts[i] = "(" + bindPlaceholders(cond, len(args)) + ")"
		args = append(args, t.args[i]...)
	}
	return " WHERE " + strings.Join(parts, " AND "), args
}

// andWhere is where with one more condition, already in SQL.
func andWhere(where, condition string) string {
	if where == "" {
		return " WHERE " + condition
	}
	return where + " AND " + condition
}

func (t Table[T]) pagingSQL() string {
	sql := ""
	if len(t.order) > 0 {
		sql += " ORDER BY " + strings.Join(t.order, ", ")
	}
	if t.limit >= 0 {
		sql += " LIMIT " + strconv.Itoa(t.limit)
	}
	if t.offset > 0 {
		sql += " OFFSET " + strconv.Itoa(t.offset)
	}
	return sql
}

// paged reports whether the query has OrderBy, Limit or Offset.
func (t Table[T]) paged() bool { return len(t.order) > 0 || t.limit >= 0 || t.offset > 0 }

// upsertSQL inserts a row of columns (bound to $1..$n, each followed by its
// cast) or, on a conflicting id, updates every column but id and created_on.
// Save documents the statement.
func upsertSQL(table string, columns []column, casts []string) string {
	names := make([]string, len(columns))
	values := make([]string, len(columns))
	var updates []string
	for i, c := range columns {
		names[i] = c.quoted()
		values[i] = "$" + strconv.Itoa(i+1) + casts[i]
		if c.name != idColumn && c.name != createdOnColumn {
			updates = append(updates, c.quoted()+" = EXCLUDED."+c.quoted())
		}
	}
	return "INSERT INTO " + table + " (" + strings.Join(names, ", ") + ") VALUES (" + strings.Join(values, ", ") +
		") ON CONFLICT (" + idColumn + ") DO UPDATE SET " + strings.Join(updates, ", ")
}

// countPlaceholders counts the ? placeholders in sql.
func countPlaceholders(sql string) int {
	n := 0
	scanPlaceholders(sql, func() string { n++; return "" })
	return n
}

// bindPlaceholders replaces each ? in sql with $k, numbering from after+1.
func bindPlaceholders(sql string, after int) string {
	return scanPlaceholders(sql, func() string { after++; return "$" + strconv.Itoa(after) })
}

// scanPlaceholders rewrites sql, replacing each placeholder ? with next() and
// each ?? with ?. Question marks inside quoted strings, quoted identifiers and
// comments are left alone.
func scanPlaceholders(sql string, next func() string) string {
	var b strings.Builder
	for i := 0; i < len(sql); i++ {
		ch := sql[i]
		switch {
		case ch == '\'' || ch == '"':
			end := closingQuote(sql, i)
			b.WriteString(sql[i:end])
			i = end - 1
		case ch == '-' && strings.HasPrefix(sql[i:], "--"):
			end := strings.IndexByte(sql[i:], '\n')
			if end < 0 {
				end = len(sql) - i
			}
			b.WriteString(sql[i : i+end])
			i += end - 1
		case ch == '/' && strings.HasPrefix(sql[i:], "/*"):
			end := strings.Index(sql[i+2:], "*/")
			if end < 0 {
				end = len(sql) - i - 2
			} else {
				end += 2
			}
			b.WriteString(sql[i : i+2+end])
			i += 2 + end - 1
		case ch == '?' && strings.HasPrefix(sql[i:], "??"):
			b.WriteByte('?')
			i++
		case ch == '?':
			b.WriteString(next())
		default:
			b.WriteByte(ch)
		}
	}
	return b.String()
}

// closingQuote returns the index just past the quoted string or identifier
// starting at i; a doubled quote is an escaped quote.
func closingQuote(sql string, i int) int {
	q := sql[i]
	for j := i + 1; j < len(sql); j++ {
		if sql[j] == q {
			if j+1 < len(sql) && sql[j+1] == q {
				j++
				continue
			}
			return j + 1
		}
	}
	return len(sql)
}

// quoteLiteral quotes s as an SQL string literal.
func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
