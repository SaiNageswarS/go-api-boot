package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-api-boot/odm/internal/core"
	"github.com/jackc/pgx/v5"
)

// Vector search with pgvector: VectorIndexSpecs become vector(N) or
// halfvec(N) columns with HNSW indexes.

// vectorSearch is VectorSearch.
func (t Table[T]) vectorSearch(ctx context.Context, index string, embedding []float32, k int) ([]odm.SearchHit[T], error) {
	if err := t.searchOnly("VectorSearch"); err != nil {
		return nil, err
	}
	spec, err := core.VectorIndex[T](index)
	if err != nil {
		return nil, err
	}
	if err := core.CheckVectorQuery(spec, embedding, k); err != nil {
		return nil, err
	}
	vec, err := pgVectorFor(spec)
	if err != nil {
		return nil, err
	}
	s, err := schemaOf[T]()
	if err != nil {
		return nil, err
	}

	col := pgx.Identifier{spec.Path}.Sanitize()
	where, args := t.whereSQL([]any{vectorLiteral(embedding)})
	where = andWhere(where, col+" IS NOT NULL")
	distance := col + " " + vec.operator + " $1::" + vec.sqlType
	sql := "SELECT " + s.selectList() + ", " + distance + " AS distance FROM " + t.qualified() + where +
		" ORDER BY distance LIMIT " + strconv.Itoa(k)

	// Session settings for this query only, applied in a transaction.
	var settings []string
	if k > 40 { // hnsw.ef_search defaults to 40, which caps the results
		settings = append(settings, "SET LOCAL hnsw.ef_search = "+strconv.Itoa(min(k, 1000)))
	}
	if len(t.where) > 0 {
		iterative, err := t.db.iterativeScans(ctx)
		if err != nil {
			return nil, err
		}
		if iterative {
			// Keep scanning the index until k rows pass the conditions.
			// relaxed_order may return rows slightly out of order, so re-sort.
			settings = append(settings, "SET LOCAL hnsw.iterative_scan = relaxed_order")
			sql = "SELECT * FROM (" + sql + ") AS s ORDER BY distance"
		}
	}
	if len(settings) == 0 {
		return t.queryHits(ctx, t.db.pool, s, sql, args, vec.score)
	}

	tx, err := t.db.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, setting := range settings {
		if _, err := tx.Exec(ctx, setting); err != nil {
			return nil, err
		}
	}
	return t.queryHits(ctx, tx, s, sql, args, vec.score)
}

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

// vectorLiteral is v as pgvector's text input, e.g. [1,0.5,0].
func vectorLiteral(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = strconv.FormatFloat(float64(f), 'g', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
