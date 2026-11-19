package odm

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Documents are stored in Postgres as relaxed Extended JSON produced from the
// model's bson encoding. Two values are rewritten on the way in:
//   - bson vectors become plain number arrays, so pgvector can cast them;
//   - dates get a fixed-width millisecond format, so they compare in order.
// On the way out, arrays are turned back into bson.Vector for model fields of
// that type.

const pgDateLayout = "2006-01-02T15:04:05.000Z"

// bsonToJSON encodes any bson-marshallable document as JSON for a jsonb column.
func bsonToJSON(doc any) (string, error) {
	raw, err := bson.Marshal(doc)
	if err != nil {
		return "", err
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		return "", err
	}
	out, err := bson.MarshalExtJSON(normalizeForPg(d), false, false)
	return string(out), err
}

// jsonValue encodes a single value the way it would appear inside a stored document.
func jsonValue(v any) (string, error) {
	doc, err := bsonToJSON(bson.D{{Key: "v", Value: v}})
	if err != nil {
		return "", err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(doc), &m); err != nil {
		return "", err
	}
	return string(m["v"]), nil
}

func normalizeForPg(v any) any {
	switch x := v.(type) {
	case bson.D:
		out := make(bson.D, len(x))
		for i, e := range x {
			out[i] = bson.E{Key: e.Key, Value: normalizeForPg(e.Value)}
		}
		return out
	case bson.A:
		out := make(bson.A, len(x))
		for i, e := range x {
			out[i] = normalizeForPg(e)
		}
		return out
	case bson.Binary:
		if x.Subtype != bson.TypeBinaryVector {
			return x
		}
		vec, err := bson.NewVectorFromBinary(x)
		if err != nil {
			return x
		}
		if f, ok := vec.Float32OK(); ok {
			arr := make(bson.A, len(f))
			for i, n := range f {
				arr[i] = float64(n)
			}
			return arr
		}
		if n, ok := vec.Int8OK(); ok {
			arr := make(bson.A, len(n))
			for i, e := range n {
				arr[i] = int32(e)
			}
			return arr
		}
		return x
	case bson.DateTime:
		t := x.Time().UTC()
		if t.Year() < 1970 || t.Year() > 9999 {
			return x
		}
		return bson.D{{Key: "$date", Value: t.Format(pgDateLayout)}}
	}
	return v
}

// jsonToModel decodes a stored jsonb document into a model.
func jsonToModel[T any](data []byte) (*T, error) {
	var d bson.D
	if err := bson.UnmarshalExtJSON(data, false, &d); err != nil {
		return nil, err
	}
	restoreVectors[T](d)

	raw, err := bson.Marshal(d)
	if err != nil {
		return nil, err
	}
	model := new(T)
	return model, bson.Unmarshal(raw, model)
}

var vectorFieldCache sync.Map // reflect.Type -> map[string]struct{}

var bsonVectorType = reflect.TypeFor[bson.Vector]()

// vectorFields lists the top-level bson keys of T whose Go type is bson.Vector.
func vectorFields[T any]() map[string]struct{} {
	t := reflect.TypeFor[T]()
	if cached, ok := vectorFieldCache.Load(t); ok {
		return cached.(map[string]struct{})
	}

	fields := map[string]struct{}{}
	if t.Kind() == reflect.Struct {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() || f.Type != bsonVectorType {
				continue
			}
			name, _, _ := strings.Cut(f.Tag.Get("bson"), ",")
			if name == "-" {
				continue
			}
			if name == "" {
				name = strings.ToLower(f.Name)
			}
			fields[name] = struct{}{}
		}
	}
	vectorFieldCache.Store(t, fields)
	return fields
}

func restoreVectors[T any](d bson.D) {
	fields := vectorFields[T]()
	if len(fields) == 0 {
		return
	}
	for i, e := range d {
		if _, ok := fields[e.Key]; !ok {
			continue
		}
		arr, ok := e.Value.(bson.A)
		if !ok {
			continue
		}
		f := make([]float32, len(arr))
		for j, n := range arr {
			f[j] = toFloat32(n)
		}
		d[i].Value = bson.NewVector(f).Binary()
	}
}

func toFloat32(v any) float32 {
	switch n := v.(type) {
	case float64:
		return float32(n)
	case int32:
		return float32(n)
	case int64:
		return float32(n)
	}
	return 0
}
