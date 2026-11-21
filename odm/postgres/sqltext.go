package postgres

import (
	"strconv"
	"strings"
)

// SQL text helpers: ? placeholders and string literals.

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
