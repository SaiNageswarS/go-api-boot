package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-api-boot/odm/internal/asyncx"
	"github.com/SaiNageswarS/go-collection-boot/async"
)

// VectorSearch runs a pgvector HNSW search. Scores follow Atlas: cosine and
// dotProduct give (1 + similarity) / 2, euclidean gives 1 / (1 + distance).
func (c *collection[T]) VectorSearch(ctx context.Context, embedding []float32, params odm.VectorSearchParams) <-chan async.Result[[]odm.SearchHit[T]] {
	return asyncx.Go(func() ([]odm.SearchHit[T], error) {
		if err := params.Validate(embedding); err != nil {
			return nil, err
		}
		spec, err := odm.VectorIndexOf[T](params.IndexName)
		if err != nil {
			return nil, err
		}
		if spec.Dimensions != len(embedding) {
			return nil, fmt.Errorf("odm/postgres: index %s has %d dimensions, query has %d", spec.Name, spec.Dimensions, len(embedding))
		}
		vec, err := pgVectorFor(spec)
		if err != nil {
			return nil, err
		}

		q := &pgQuery{}
		where, err := q.where(params.Filter)
		if err != nil {
			return nil, err
		}
		distance := vec.expr + " " + vec.operator + " " + q.arg(vectorLiteral(embedding)) + "::" + vec.sqlType
		sql := "SELECT doc::text, " + distance + " AS distance FROM " + c.qualified() +
			" WHERE " + vec.predicate + " AND " + where +
			" ORDER BY distance LIMIT " + strconv.Itoa(params.K)

		// Session settings for this query only, applied in a transaction.
		var settings []string
		if params.NumCandidates > 0 {
			settings = append(settings, "SET LOCAL hnsw.ef_search = "+strconv.Itoa(min(max(params.NumCandidates, params.K), 1000)))
		}
		if len(params.Filter) > 0 && c.db.iterativeScans(ctx) {
			// Keep scanning the index until K rows pass the filter. relaxed_order
			// may return rows slightly out of order, so re-sort them.
			settings = append(settings, "SET LOCAL hnsw.iterative_scan = relaxed_order")
			sql = "SELECT * FROM (" + sql + ") AS s ORDER BY distance"
		}
		if len(settings) == 0 {
			return queryHits[T](ctx, c.db.pool, sql, q.args, vec.score)
		}

		tx, err := c.db.pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		for _, s := range settings {
			if _, err := tx.Exec(ctx, s); err != nil {
				return nil, err
			}
		}
		return queryHits[T](ctx, tx, sql, q.args, vec.score)
	})
}

// pgVector describes how a vector field is cast, compared and indexed. Index
// definitions and queries share it so the planner matches them.
type pgVector struct {
	expr      string // ((doc #>> '{path}')::vector(N))
	predicate string // rows the (partial) index covers: arrays of N numbers
	sqlType   string // vector(N) or halfvec(N)
	operator  string // <=>, <#>, <->
	opClass   string // HNSW operator class
	score     func(distance float64) float64
}

// pgVectorFor maps a spec onto pgvector. Scalar quantization, or more than
// 2000 dimensions (the HNSW limit for vector), uses halfvec.
func pgVectorFor(spec odm.VectorIndexSpec) (pgVector, error) {
	if spec.Path == "" || spec.Dimensions <= 0 {
		return pgVector{}, fmt.Errorf("odm/postgres: vector index %q needs a path and dimensions", spec.Name)
	}
	base := "vector"
	switch {
	case spec.Quantization == "binary":
		return pgVector{}, errUnsupported("vector index %s: binary quantization", spec.Name)
	case spec.Quantization == "scalar" || spec.Dimensions > 2000:
		base = "halfvec"
	case spec.Quantization != "":
		return pgVector{}, errUnsupported("vector index %s: quantization %q", spec.Name, spec.Quantization)
	}

	path := strings.Split(spec.Path, ".")
	p := jsonbExpr(path)
	v := pgVector{sqlType: fmt.Sprintf("%s(%d)", base, spec.Dimensions)}
	v.expr = "(" + textExpr(path) + "::" + v.sqlType + ")"
	// Documents without a usable embedding stay out of the index (and out of
	// results) instead of failing the write.
	v.predicate = fmt.Sprintf("(jsonb_typeof(%s) = 'array' AND jsonb_array_length(%s) = %d)", p, p, spec.Dimensions)

	switch spec.SimilarityOrDefault() {
	case odm.Cosine: // <=> is 1 - cosine similarity
		v.operator, v.opClass = "<=>", base+"_cosine_ops"
		v.score = func(d float64) float64 { return 1 - d/2 }
	case odm.DotProduct: // <#> is the negative inner product
		v.operator, v.opClass = "<#>", base+"_ip_ops"
		v.score = func(d float64) float64 { return (1 - d) / 2 }
	case odm.Euclidean:
		v.operator, v.opClass = "<->", base+"_l2_ops"
		v.score = func(d float64) float64 { return 1 / (1 + d) }
	default:
		return pgVector{}, fmt.Errorf("odm/postgres: vector index %s: unknown similarity %q", spec.Name, spec.Similarity)
	}
	return v, nil
}

func vectorLiteral(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = strconv.FormatFloat(float64(f), 'g', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
