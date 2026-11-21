package postgres

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-api-boot/odm/internal/core"
	"github.com/jackc/pgx/v5"
)

// The SQL of vector search with pgvector: VectorIndexSpecs become vector(N)
// or halfvec(N) columns with HNSW indexes.

// pgVector describes how a vector column is typed, compared and indexed.
type pgVector struct {
	sqlType  string // vector(N) or halfvec(N)
	operator string // <=>, <#>, <->
	opClass  string // HNSW operator class
	score    func(distance float64) float64
}

// pgVectorFor maps a spec onto pgvector. Scalar quantization, or more than
// 2000 dimensions (the HNSW limit for vector), uses halfvec.
func pgVectorFor(spec odm.VectorIndexSpec) (pgVector, error) {
	if err := core.ValidateVectorIndex(spec); err != nil {
		return pgVector{}, err
	}
	base := "vector"
	switch {
	case spec.Quantization == "binary":
		return pgVector{}, core.Unsupported("vector index %s: binary quantization", spec.Name)
	case spec.Quantization == "scalar" || spec.Dimensions > 2000:
		base = "halfvec"
	case spec.Quantization != "":
		return pgVector{}, core.Unsupported("vector index %s: quantization %q", spec.Name, spec.Quantization)
	}

	v := pgVector{sqlType: fmt.Sprintf("%s(%d)", base, spec.Dimensions)}
	switch core.Similarity(spec) {
	case odm.Cosine: // <=> is 1 - cosine similarity
		v.operator, v.opClass = "<=>", base+"_cosine_ops"
		v.score = func(d float64) float64 { return 1 - d/2 }
	case odm.DotProduct: // <#> is the negative inner product
		v.operator, v.opClass = "<#>", base+"_ip_ops"
		v.score = func(d float64) float64 { return (1 - d) / 2 }
	case odm.Euclidean:
		v.operator, v.opClass = "<->", base+"_l2_ops"
		v.score = func(d float64) float64 { return 1 / (1 + d) }
	}
	return v, nil
}

// hnswIndexSQL is the definition of spec's HNSW index on table.
func hnswIndexSQL(table string, spec odm.VectorIndexSpec, vec pgVector) string {
	return "ON " + table + " USING hnsw (" + pgx.Identifier{spec.Path}.Sanitize() + " " + vec.opClass + ")"
}

// vectorSearchSQL selects the k rows of s nearest to embedding in spec's
// column, among those matching t's conditions, each followed by its distance.
// An iterative scan may return rows slightly out of order, so it re-sorts.
func (t Table[T]) vectorSearchSQL(s *tableSchema, spec odm.VectorIndexSpec, vec pgVector, embedding []float32, k int, iterative bool) (string, []any) {
	col := pgx.Identifier{spec.Path}.Sanitize()
	where, args := t.whereSQL([]any{vectorLiteral(embedding)})
	where = andWhere(where, col+" IS NOT NULL")
	distance := col + " " + vec.operator + " $1::" + vec.sqlType
	sql := "SELECT " + s.selectList() + ", " + distance + " AS distance FROM " + t.qualified() + where +
		" ORDER BY distance LIMIT " + strconv.Itoa(k)
	if iterative {
		sql = "SELECT * FROM (" + sql + ") AS s ORDER BY distance"
	}
	return sql, args
}

// vectorSearchSettings are the session settings a search for k rows needs.
func vectorSearchSettings(k int, iterative bool) []string {
	var settings []string
	if k > 40 { // hnsw.ef_search defaults to 40, which caps the results
		settings = append(settings, "SET LOCAL hnsw.ef_search = "+strconv.Itoa(min(k, 1000)))
	}
	if iterative { // keep scanning the index until k rows pass the conditions
		settings = append(settings, "SET LOCAL hnsw.iterative_scan = relaxed_order")
	}
	return settings
}

// vectorLiteral is v as pgvector's text input, e.g. [1,0.5,0].
func vectorLiteral(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = strconv.FormatFloat(float64(f), 'g', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
