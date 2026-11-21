package postgres

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-api-boot/odm/internal/core"
	"github.com/jackc/pgx/v5"
)

type columnKind int

const (
	plainColumn  columnKind = iota // pgx encodes and decodes the field directly
	arrayColumn                    // nil slices are written as empty arrays
	jsonColumn                     // encoding/json
	vectorColumn                   // pgvector, written as a literal and read as real[]
)

// The columns the ORM itself writes.
const (
	idColumn        = "id"         // the primary key, from Model.Id()
	createdOnColumn = "created_on" // stamped by Save on insert
	updatedOnColumn = "updated_on" // stamped by Save on every save
)

type column struct {
	name     string
	index    []int // reflect field index; nil for a timestamp the model doesn't map
	typ      reflect.Type
	sqlType  string // as format_type reports it, e.g. "bigint", "text[]", "vector(3)"
	kind     columnKind
	nullable bool
	dflt     string // DEFAULT literal for NOT NULL columns
}

func (c column) quoted() string { return pgx.Identifier{c.name}.Sanitize() }

// selectExpr is how the column is read: vectors as real[], which pgx decodes
// into []float32.
func (c column) selectExpr() string {
	if c.kind != vectorColumn {
		return c.quoted()
	}
	if strings.HasPrefix(c.sqlType, "halfvec") {
		return c.quoted() + "::vector::real[]"
	}
	return c.quoted() + "::real[]"
}

// definition is the column's DDL, as used by CREATE TABLE and ADD COLUMN.
func (c column) definition() string {
	def := c.quoted() + " " + c.sqlType
	if c.name == idColumn {
		return def + " PRIMARY KEY"
	}
	if !c.nullable {
		def += " NOT NULL DEFAULT " + c.dflt
	}
	return def
}

// arg returns the value to bind for model's field and the placeholder cast.
func (c column) arg(model reflect.Value) (any, string, error) {
	v := model.FieldByIndex(c.index)
	switch c.kind {
	case arrayColumn:
		if v.IsNil() {
			return reflect.MakeSlice(c.typ, 0, 0).Interface(), "", nil
		}
	case jsonColumn:
		js, err := json.Marshal(v.Interface())
		if err != nil {
			return nil, "", fmt.Errorf("encoding column %s: %w", c.name, err)
		}
		return string(js), "::jsonb", nil
	case vectorColumn:
		if v.Len() == 0 {
			return nil, "::" + c.sqlType, nil
		}
		return vectorLiteral(v.Interface().([]float32)), "::" + c.sqlType, nil
	}
	if c.nullable && v.IsNil() {
		return nil, "", nil
	}
	return v.Interface(), "", nil
}

// scanTarget returns what to pass to Scan for the field, and a function that
// finishes decoding after the scan.
func (c column) scanTarget(model reflect.Value) (any, func() error) {
	field := model.FieldByIndex(c.index)
	if c.kind != jsonColumn {
		return field.Addr().Interface(), nil
	}
	var raw []byte
	return &raw, func() error {
		if len(raw) == 0 {
			return nil
		}
		if err := json.Unmarshal(raw, field.Addr().Interface()); err != nil {
			return fmt.Errorf("decoding column %s: %w", c.name, err)
		}
		return nil
	}
}

type tableSchema struct {
	columns []column // every column of the table, in DDL order
	byName  map[string]*column
}

// fields are the columns mapped to model fields: what reads select and scan.
func (s *tableSchema) fields() []column {
	return slices.DeleteFunc(slices.Clone(s.columns), func(c column) bool { return c.index == nil })
}

// selectList is the SELECT list that reads a model.
func (s *tableSchema) selectList() string {
	fields := s.fields()
	exprs := make([]string, len(fields))
	for i, c := range fields {
		exprs[i] = c.selectExpr()
	}
	return strings.Join(exprs, ", ")
}

var schemaCache sync.Map // reflect.Type -> *tableSchema

// schemaOf maps T's fields to columns, plus the timestamp columns the model
// doesn't map, failing on models the backend can't store.
func schemaOf[T odm.Model]() (*tableSchema, error) {
	t := reflect.TypeFor[T]()
	if cached, ok := schemaCache.Load(t); ok {
		return cached.(*tableSchema), nil
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("model %s must be a struct", t)
	}

	vectors := map[string]odm.VectorIndexSpec{}
	for _, spec := range core.VectorIndexes[T]() {
		vectors[spec.Path] = spec
	}

	s := &tableSchema{byName: map[string]*column{}}
	var walk func(t reflect.Type, index []int) error
	walk = func(t reflect.Type, index []int) error {
		for i := range t.NumField() {
			f := t.Field(i)
			tag, hasTag := f.Tag.Lookup("db")
			name, _, _ := strings.Cut(tag, ",")
			if name == "-" {
				continue
			}
			idx := append(append([]int(nil), index...), i)
			// Embedded structs flatten, even unexported ones (as in encoding/json).
			if f.Anonymous && !hasTag && f.Type.Kind() == reflect.Struct {
				if err := walk(f.Type, idx); err != nil {
					return err
				}
				continue
			}
			if !f.IsExported() {
				continue
			}
			if name == "" {
				name = snakeCase(f.Name)
			}
			if _, dup := s.byName[name]; dup {
				return fmt.Errorf("model %s maps two fields to column %q", t, name)
			}
			col, err := columnFor(name, f.Type, vectors)
			if err != nil {
				return fmt.Errorf("model %s, field %s: %w", t, f.Name, err)
			}
			col.index = idx
			s.columns = append(s.columns, col)
			s.byName[name] = &s.columns[len(s.columns)-1]
		}
		return nil
	}
	if err := walk(t, nil); err != nil {
		return nil, err
	}
	for _, name := range []string{createdOnColumn, updatedOnColumn} {
		if c, ok := s.byName[name]; ok {
			if c.typ.Kind() != reflect.Int64 {
				return nil, fmt.Errorf("model %s: column %q is stamped by Save and must be an int64 field, not %s", t, name, c.typ)
			}
			continue
		}
		s.columns = append(s.columns, column{name: name, typ: reflect.TypeFor[int64](), sqlType: "bigint", dflt: "0"})
	}
	// Rebuild the pointers: appending may have moved the slice.
	for i := range s.columns {
		s.byName[s.columns[i].name] = &s.columns[i]
	}

	id, ok := s.byName[idColumn]
	if !ok || id.typ.Kind() != reflect.String {
		return nil, fmt.Errorf("model %s needs a string field for the %q column", t, idColumn)
	}
	for path, spec := range vectors {
		if c, ok := s.byName[path]; !ok || c.kind != vectorColumn {
			return nil, fmt.Errorf("vector index %s: model %s has no []float32 column %q", spec.Name, t, path)
		}
	}

	cached, _ := schemaCache.LoadOrStore(t, s)
	return cached.(*tableSchema), nil
}

var timeType = reflect.TypeFor[time.Time]()

func columnFor(name string, t reflect.Type, vectors map[string]odm.VectorIndexSpec) (column, error) {
	c := column{name: name, typ: t}

	if spec, ok := vectors[name]; ok {
		if t != reflect.TypeFor[[]float32]() {
			return c, fmt.Errorf("vector index %s needs a []float32 field, not %s", spec.Name, t)
		}
		vec, err := pgVectorFor(spec)
		if err != nil {
			return c, err
		}
		c.sqlType, c.kind, c.nullable = vec.sqlType, vectorColumn, true
		return c, nil
	}

	if t.Kind() == reflect.Pointer {
		inner, err := columnFor(name, t.Elem(), nil)
		if err != nil {
			return c, err
		}
		if inner.kind == jsonColumn {
			// encoding/json handles the pointer itself.
			inner.typ = t
			inner.nullable, inner.dflt = false, "'null'"
			return inner, nil
		}
		// pgx encodes a nil pointer as NULL and decodes NULL into nil.
		inner.typ, inner.kind, inner.nullable, inner.dflt = t, plainColumn, true, ""
		return inner, nil
	}

	if sqlType, dflt, ok := scalarType(t); ok {
		c.sqlType, c.dflt = sqlType, dflt
		return c, nil
	}
	if t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		if elem, _, ok := scalarType(t.Elem()); ok && t.Kind() == reflect.Slice && t.Elem().Kind() != reflect.Uint8 {
			c.sqlType, c.kind, c.dflt = elem+"[]", arrayColumn, "'{}'"
			return c, nil
		}
	}
	c.sqlType, c.kind, c.dflt = "jsonb", jsonColumn, "'null'"
	return c, nil
}

// scalarType maps a Go type to a scalar SQL type and its zero-value default.
func scalarType(t reflect.Type) (string, string, bool) {
	if t == timeType {
		return "timestamp with time zone", "'0001-01-01 00:00:00+00'", true
	}
	switch t.Kind() {
	case reflect.String:
		return "text", "''", true
	case reflect.Bool:
		return "boolean", "false", true
	case reflect.Int, reflect.Int64, reflect.Uint, reflect.Uint32, reflect.Uint64:
		return "bigint", "0", true
	case reflect.Int32, reflect.Uint16:
		return "integer", "0", true
	case reflect.Int16, reflect.Int8, reflect.Uint8:
		return "smallint", "0", true
	case reflect.Float64:
		return "double precision", "0", true
	case reflect.Float32:
		return "real", "0", true
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return "bytea", `'\x'`, true
		}
	}
	return "", "", false
}

// snakeCase turns a Go field name into a column name: ReleasedOn -> released_on,
// HTTPServer -> http_server.
func snakeCase(s string) string {
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 && (unicode.IsLower(runes[i-1]) || (i+1 < len(runes) && unicode.IsLower(runes[i+1]))) {
				b.WriteByte('_')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}
