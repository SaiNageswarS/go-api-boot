package postgres

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// pgQuery compiles MongoDB-style filters and sorts into SQL over the
// "doc" jsonb column, collecting positional arguments as it goes.
//
// Supported:
//   - equality on any (dotted) field, including array membership for scalars
//   - $eq $ne $gt $gte $lt $lte $in $nin $exists $regex/$options $size $all
//     $not $elemMatch, and the logical $and $or $nor
//
// Differences from MongoDB worth knowing:
//   - equality with a sub-document is containment (the stored sub-document
//     may have extra fields);
//   - dotted paths do not reach into arrays of sub-documents (use $elemMatch);
//   - $gt/$lt only compare values of the same JSON type;
//   - $regex doesn't match inside arrays.
type pgQuery struct {
	args []any
	// depth > 0 inside $elemMatch, where "doc" is an array element and _id
	// is an ordinary field, not the row's id column.
	depth int
}

func (q *pgQuery) arg(v any) string {
	q.args = append(q.args, v)
	return fmt.Sprintf("$%d", len(q.args))
}

// where compiles a filter document into a boolean SQL expression.
func (q *pgQuery) where(filter any) (string, error) {
	pairs, ok := toPairs(filter)
	if !ok {
		return "", errUnsupported("filter type %T", filter)
	}
	if len(pairs) == 0 {
		return "TRUE", nil
	}

	parts := make([]string, 0, len(pairs))
	for _, e := range pairs {
		var (
			clause string
			err    error
		)
		switch e.Key {
		case "$and", "$or", "$nor":
			clause, err = q.logical(e.Key, e.Value)
		default:
			if strings.HasPrefix(e.Key, "$") {
				return "", errUnsupported("operator %s", e.Key)
			}
			clause, err = q.field(strings.Split(e.Key, "."), e.Value)
		}
		if err != nil {
			return "", err
		}
		parts = append(parts, clause)
	}
	return "(" + strings.Join(parts, " AND ") + ")", nil
}

func (q *pgQuery) logical(op string, v any) (string, error) {
	subs, ok := toSlice(v)
	if !ok || len(subs) == 0 {
		return "", fmt.Errorf("odm/postgres: %s needs a non-empty array", op)
	}
	parts := make([]string, len(subs))
	for i, s := range subs {
		clause, err := q.where(s)
		if err != nil {
			return "", err
		}
		parts[i] = clause
	}
	switch op {
	case "$and":
		return "(" + strings.Join(parts, " AND ") + ")", nil
	case "$or":
		return "(" + strings.Join(parts, " OR ") + ")", nil
	default: // $nor
		return "(NOT COALESCE(" + strings.Join(parts, " OR ") + ", false))", nil
	}
}

// field compiles the condition on one field: a literal value or an operator document.
func (q *pgQuery) field(path []string, v any) (string, error) {
	if re, ok := v.(bson.Regex); ok {
		return q.regex(path, re.Pattern, re.Options)
	}

	ops, ok := operatorPairs(v)
	if !ok {
		return q.eq(path, v)
	}

	regexOptions := ""
	for _, op := range ops {
		if op.Key == "$options" {
			s, ok := op.Value.(string)
			if !ok {
				return "", fmt.Errorf("odm/postgres: $options must be a string")
			}
			regexOptions = s
		}
	}

	parts := make([]string, 0, len(ops))
	for _, op := range ops {
		var (
			clause string
			err    error
		)
		switch op.Key {
		case "$options":
			continue
		case "$eq":
			clause, err = q.eq(path, op.Value)
		case "$ne":
			clause, err = q.eq(path, op.Value)
			clause = negate(clause)
		case "$gt", "$gte", "$lt", "$lte":
			clause, err = q.cmp(path, op.Key, op.Value)
		case "$in":
			clause, err = q.in(path, op.Value)
		case "$nin":
			clause, err = q.in(path, op.Value)
			clause = negate(clause)
		case "$exists":
			clause = q.exists(path, truthy(op.Value))
		case "$regex":
			switch re := op.Value.(type) {
			case string:
				clause, err = q.regex(path, re, regexOptions)
			case bson.Regex:
				opts := re.Options
				if regexOptions != "" {
					opts = regexOptions
				}
				clause, err = q.regex(path, re.Pattern, opts)
			default:
				err = fmt.Errorf("odm/postgres: $regex must be a string or bson.Regex")
			}
		case "$size":
			clause, err = q.size(path, op.Value)
		case "$all":
			clause, err = q.all(path, op.Value)
		case "$not":
			clause, err = q.field(path, op.Value)
			clause = negate(clause)
		case "$elemMatch":
			clause, err = q.elemMatch(path, op.Value)
		default:
			err = errUnsupported("operator %s", op.Key)
		}
		if err != nil {
			return "", err
		}
		parts = append(parts, clause)
	}
	if len(parts) == 0 {
		return "TRUE", nil
	}
	return "(" + strings.Join(parts, " AND ") + ")", nil
}

func (q *pgQuery) eq(path []string, v any) (string, error) {
	if re, ok := v.(bson.Regex); ok {
		return q.regex(path, re.Pattern, re.Options)
	}
	if s, ok := v.(string); ok && q.isRowID(path) {
		return "(id = " + q.arg(s) + ")", nil
	}
	if v == nil {
		p := jsonbExpr(path)
		return fmt.Sprintf("(%s IS NULL OR %s = 'null'::jsonb)", p, p), nil
	}

	variants, err := containmentVariants(path, v)
	if err != nil {
		return "", err
	}
	parts := make([]string, len(variants))
	for i, variant := range variants {
		parts[i] = "doc @> " + q.arg(variant) + "::jsonb"
	}
	return "(" + strings.Join(parts, " OR ") + ")", nil
}

// maxArrayPathDepth bounds how many leading path segments may pass through
// arrays: each one doubles the number of containment checks.
const maxArrayPathDepth = 3

// containmentVariants returns the JSON documents that, contained in doc, mean
// path equals v the way MongoDB matches it: any segment may be an array of
// sub-documents, and a scalar also matches an array holding it. For
// {"cast.name": "Al"} that is {"cast":{"name":"Al"}}, {"cast":{"name":["Al"]}},
// {"cast":[{"name":"Al"}]} and {"cast":[{"name":["Al"]}]}. Each variant is
// served by the GIN index on doc.
func containmentVariants(path []string, v any) ([]string, error) {
	whole, err := jsonValue(v)
	if err != nil {
		return nil, err
	}
	if len(path) == 0 { // inside $elemMatch: the element itself
		return []string{whole}, nil
	}
	leaves := []any{v}
	if !strings.HasPrefix(whole, "{") && !strings.HasPrefix(whole, "[") {
		leaves = append(leaves, bson.A{v})
	}

	var out []string
	for _, leaf := range leaves {
		for _, doc := range nestings(path, leaf) {
			js, err := bsonToJSON(doc)
			if err != nil {
				return nil, err
			}
			out = append(out, js)
		}
	}
	return out, nil
}

// nestings wraps leaf under path, once per choice of which of the first
// maxArrayPathDepth segments hold an array of sub-documents.
func nestings(path []string, leaf any) []bson.D {
	values := []any{leaf}
	for i := len(path) - 1; i >= 0; i-- {
		next := make([]any, 0, 2*len(values))
		for _, inner := range values {
			next = append(next, bson.D{{Key: path[i], Value: inner}})
		}
		if i > 0 && i <= maxArrayPathDepth {
			// path[i-1] may hold an array of these sub-documents.
			for _, obj := range next[:len(values)] {
				next = append(next, bson.A{obj})
			}
		}
		values = next
	}
	docs := make([]bson.D, len(values))
	for i, v := range values {
		docs[i] = v.(bson.D)
	}
	return docs
}

var pgComparison = map[string]string{"$gt": ">", "$gte": ">=", "$lt": "<", "$lte": "<="}

func (q *pgQuery) cmp(path []string, op string, v any) (string, error) {
	sqlOp := pgComparison[op]
	if s, ok := v.(string); ok && q.isRowID(path) {
		return fmt.Sprintf("(id %s %s)", sqlOp, q.arg(s)), nil
	}
	val, err := jsonValue(v)
	if err != nil {
		return "", err
	}
	p, a := jsonbExpr(path), q.arg(val)
	return fmt.Sprintf("(jsonb_typeof(%s) = jsonb_typeof(%s::jsonb) AND %s %s %s::jsonb)", p, a, p, sqlOp, a), nil
}

func (q *pgQuery) in(path []string, v any) (string, error) {
	vals, ok := toSlice(v)
	if !ok {
		return "", fmt.Errorf("odm/postgres: $in/$nin needs an array")
	}
	if len(vals) == 0 {
		return "FALSE", nil
	}

	if q.isRowID(path) {
		ids := make([]string, 0, len(vals))
		for _, val := range vals {
			if s, ok := val.(string); ok {
				ids = append(ids, s)
			}
		}
		if len(ids) == len(vals) {
			return "(id = ANY(" + q.arg(ids) + "::text[]))", nil
		}
	}

	parts := make([]string, len(vals))
	for i, val := range vals {
		clause, err := q.eq(path, val)
		if err != nil {
			return "", err
		}
		parts[i] = clause
	}
	return "(" + strings.Join(parts, " OR ") + ")", nil
}

func (q *pgQuery) exists(path []string, want bool) string {
	if q.isRowID(path) {
		return strings.ToUpper(fmt.Sprint(want))
	}
	if want {
		return "(" + jsonbExpr(path) + " IS NOT NULL)"
	}
	return "(" + jsonbExpr(path) + " IS NULL)"
}

// regex maps MongoDB regex options to Postgres embedded options. Postgres's
// default lets "." match newlines and anchors match only at the ends;
// MongoDB's default keeps "." off newlines. So:
//
//	(none) -> p    m -> n    s -> (none)    m+s -> w    i -> i    x -> x
func (q *pgQuery) regex(path []string, pattern, options string) (string, error) {
	var multiline, dotAll bool
	flags := ""
	for _, o := range options {
		switch o {
		case 'i':
			flags += "i"
		case 'x':
			flags += "x"
		case 'm':
			multiline = true
		case 's':
			dotAll = true
		default:
			return "", errUnsupported("regex option %q", o)
		}
	}
	switch {
	case multiline && dotAll:
		flags += "w"
	case multiline:
		flags += "n"
	case !dotAll:
		flags += "p"
	}
	if flags != "" {
		pattern = "(?" + flags + ")" + pattern
	}
	return fmt.Sprintf("(jsonb_typeof(%s) = 'string' AND %s ~ %s)", jsonbExpr(path), textExpr(path), q.arg(pattern)), nil
}

func (q *pgQuery) size(path []string, v any) (string, error) {
	n, ok := toInt64(v)
	if !ok {
		return "", fmt.Errorf("odm/postgres: $size needs a number")
	}
	p := jsonbExpr(path)
	return fmt.Sprintf("(jsonb_typeof(%s) = 'array' AND jsonb_array_length(%s) = %s)", p, p, q.arg(n)), nil
}

func (q *pgQuery) all(path []string, v any) (string, error) {
	vals, ok := toSlice(v)
	if !ok {
		return "", fmt.Errorf("odm/postgres: $all needs an array")
	}
	if len(vals) == 0 {
		return "FALSE", nil
	}
	if len(path) == 0 {
		js, err := jsonValue(bson.A(vals))
		if err != nil {
			return "", err
		}
		return "(doc @> " + q.arg(js) + "::jsonb)", nil
	}
	var parts []string
	for _, doc := range nestings(path, bson.A(vals)) {
		js, err := bsonToJSON(doc)
		if err != nil {
			return "", err
		}
		parts = append(parts, "doc @> "+q.arg(js)+"::jsonb")
	}
	return "(" + strings.Join(parts, " OR ") + ")", nil
}

// elemMatch evaluates the sub-filter against each array element, rebinding
// "doc" to the element so the same compiler applies inside.
func (q *pgQuery) elemMatch(path []string, v any) (string, error) {
	var (
		sub string
		err error
	)
	q.depth++
	if _, isOps := operatorPairs(v); isOps {
		sub, err = q.field(nil, v) // conditions on the element itself
	} else {
		sub, err = q.where(v) // conditions on fields of the element
	}
	q.depth--
	if err != nil {
		return "", err
	}
	p := jsonbExpr(path)
	return fmt.Sprintf(
		"EXISTS (SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(%s) = 'array' THEN %s ELSE '[]'::jsonb END) AS elem(doc) WHERE %s)",
		p, p, sub), nil
}

// orderBy compiles a sort. Like MongoDB, missing values sort first ascending
// and last descending.
func orderBy(fields []odm.SortField) string {
	if len(fields) == 0 {
		return ""
	}
	parts := make([]string, len(fields))
	for i, f := range fields {
		parts[i] = sortExpr(f.Field)
		if f.Desc {
			parts[i] += " DESC NULLS LAST"
		} else {
			parts[i] += " ASC NULLS FIRST"
		}
	}
	return " ORDER BY " + strings.Join(parts, ", ")
}

// sortExpr is the column or jsonb expression a field sorts and indexes on.
func sortExpr(field string) string {
	if field == "_id" {
		return "id"
	}
	return jsonbExpr(strings.Split(field, "."))
}

/* ── path expressions ── */

// isRowID reports a top-level _id, which compiles to the indexed id column.
func (q *pgQuery) isRowID(path []string) bool {
	return q.depth == 0 && len(path) == 1 && path[0] == "_id"
}

// pathLiteral renders a path as a quoted SQL text[] literal: '{"a","b"}'.
// Index definitions and queries use the same form so the planner matches them.
func pathLiteral(path []string) string {
	quoted := make([]string, len(path))
	for i, p := range path {
		p = strings.ReplaceAll(p, `\`, `\\`)
		p = strings.ReplaceAll(p, `"`, `\"`)
		quoted[i] = `"` + p + `"`
	}
	return quoteLiteral("{" + strings.Join(quoted, ",") + "}")
}

func jsonbExpr(path []string) string {
	if len(path) == 0 {
		return "doc"
	}
	return "(doc #> " + pathLiteral(path) + ")"
}

func textExpr(path []string) string {
	if len(path) == 0 {
		return "(doc #>> '{}')"
	}
	return "(doc #>> " + pathLiteral(path) + ")"
}

func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// errUnsupported reports a query or index feature the Postgres backend can't
// translate. It wraps errors.ErrUnsupported.
func errUnsupported(format string, args ...any) error {
	return fmt.Errorf("odm/postgres: %w: "+format, append([]any{errors.ErrUnsupported}, args...)...)
}

func negate(clause string) string { return "(NOT COALESCE(" + clause + ", false))" }

/* ── loose typing helpers ── */

// toPairs reads a filter/sort document as ordered key-value pairs.
// Map keys are sorted so generated SQL is deterministic.
func toPairs(v any) ([]bson.E, bool) {
	switch x := v.(type) {
	case nil:
		return nil, true
	case odm.M:
		return sortedPairs(x), true
	case bson.D:
		return x, true
	case bson.M:
		return sortedPairs(x), true
	case map[string]any:
		return sortedPairs(x), true
	case bson.Raw:
		var d bson.D
		if err := bson.Unmarshal(x, &d); err != nil {
			return nil, false
		}
		return d, true
	}
	return nil, false
}

func sortedPairs(m map[string]any) []bson.E {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]bson.E, len(keys))
	for i, k := range keys {
		out[i] = bson.E{Key: k, Value: m[k]}
	}
	return out
}

// operatorPairs returns the pairs of v if it is an operator document ({"$gt": 1}).
func operatorPairs(v any) ([]bson.E, bool) {
	pairs, ok := toPairs(v)
	if !ok || len(pairs) == 0 || !strings.HasPrefix(pairs[0].Key, "$") {
		return nil, false
	}
	return pairs, true
}

func toSlice(v any) ([]any, bool) {
	switch x := v.(type) {
	case odm.A:
		return x, true
	case bson.A:
		return x, true
	case []any:
		return x, true
	}
	rv := reflect.ValueOf(v)
	if !rv.IsValid() || (rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array) {
		return nil, false
	}
	out := make([]any, rv.Len())
	for i := range out {
		out[i] = rv.Index(i).Interface()
	}
	return out, true
}

func toInt64(v any) (int64, bool) {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(rv.Uint()), true
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		return int64(f), f == float64(int64(f))
	}
	return 0, false
}

func truthy(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	n, ok := toInt64(v)
	return !ok || n != 0
}
