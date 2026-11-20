package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/jackc/pgx/v5"
)

// VectorSearch returns the k rows nearest to embedding by the model's
// VectorIndexSpec named index, best first, among the rows matching t's
// conditions. It runs a pgvector HNSW search; with conditions it scans the
// index iteratively (pgvector 0.8+) so that filtering doesn't return fewer
// than k rows. OrderBy, Limit and Offset don't apply.
//
// Scores follow Atlas: cosine and dotProduct give (1 + similarity) / 2,
// euclidean gives 1 / (1 + distance).
func (t Table[T]) VectorSearch(ctx context.Context, index string, embedding []float32, k int) <-chan async.Result[[]odm.SearchHit[T]] {
	return async.Go(func() ([]odm.SearchHit[T], error) {
		if len(embedding) == 0 || k <= 0 {
			return nil, errors.New("odm/postgres: VectorSearch needs an embedding and k > 0")
		}
		if err := t.searchOnly("VectorSearch"); err != nil {
			return nil, err
		}
		spec, err := odm.VectorIndexOf[T](index)
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
		s, err := schemaOf[T]()
		if err != nil {
			return nil, err
		}

		col := pgx.Identifier{spec.Path}.Sanitize()
		where, args := t.whereSQL([]any{vectorLiteral(embedding)})
		if where == "" {
			where = " WHERE " + col + " IS NOT NULL"
		} else {
			where += " AND " + col + " IS NOT NULL"
		}
		distance := col + " " + vec.operator + " $1::" + vec.sqlType
		sql := "SELECT " + selectList(s) + ", " + distance + " AS distance FROM " + t.qualified() + where +
			" ORDER BY distance LIMIT " + strconv.Itoa(k)

		// Session settings for this query only, applied in a transaction.
		var settings []string
		if k > 40 { // hnsw.ef_search defaults to 40, which caps the results
			settings = append(settings, "SET LOCAL hnsw.ef_search = "+strconv.Itoa(min(k, 1000)))
		}
		if len(t.where) > 0 && t.db.iterativeScans(ctx) {
			// Keep scanning the index until k rows pass the conditions.
			// relaxed_order may return rows slightly out of order, so re-sort.
			settings = append(settings, "SET LOCAL hnsw.iterative_scan = relaxed_order")
			sql = "SELECT * FROM (" + sql + ") AS s ORDER BY distance"
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
	})
}

// searchOnly rejects paging on a search, which orders by relevance itself.
func (t Table[T]) searchOnly(method string) error {
	if t.err != nil {
		return t.err
	}
	if len(t.order) > 0 || t.limit >= 0 || t.offset > 0 {
		return fmt.Errorf("odm/postgres: %s orders by relevance and takes its own limit; drop OrderBy, Limit and Offset", method)
	}
	return nil
}

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// queryHits runs a search whose rows are the model's columns plus a
// distance or score, turned into a hit score by score.
func (t Table[T]) queryHits(ctx context.Context, q querier, s *tableSchema, sql string, args []any, score func(float64) float64) ([]odm.SearchHit[T], error) {
	rows, err := q.Query(ctx, sql, args...)
	if isMissing(err) {
		return []odm.SearchHit[T]{}, nil
	}
	if err != nil {
		return nil, t.queryErr(err, sql)
	}
	var raw []float64
	models, err := scanRows[T](rows, s, &raw)
	if err != nil {
		return nil, t.queryErr(err, sql)
	}
	hits := make([]odm.SearchHit[T], len(models))
	for i, m := range models {
		hits[i] = odm.SearchHit[T]{Score: score(raw[i]), Doc: m}
	}
	return hits, nil
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
	if spec.Path == "" || spec.Dimensions <= 0 {
		return pgVector{}, fmt.Errorf("odm/postgres: vector index %q needs a path (the column) and dimensions", spec.Name)
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

	v := pgVector{sqlType: fmt.Sprintf("%s(%d)", base, spec.Dimensions)}

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
