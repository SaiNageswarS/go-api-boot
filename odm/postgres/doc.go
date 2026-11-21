// Package postgres is go-api-boot's Postgres ORM: typed tables with SQL
// queries, plus vector search (pgvector) and BM25 term search (ParadeDB).
//
//	db, err := postgres.Open(ctx, os.Getenv("POSTGRES_URI"))
//	err = postgres.EnsureTable[Profile](ctx, db, tenant) // at deploy / startup
//
//	profiles := postgres.TableOf[Profile](db, tenant)
//	top, err := async.Await(profiles.
//	    Where("score > ?", 89).
//	    Where("city = ?", city).
//	    OrderBy("score DESC").
//	    Limit(10).
//	    Find(ctx))
//
// Storage: a tenant is a schema and a model is a table "<tenant>"."<name>"
// (the model's CollectionName). Queries are SQL over its columns.
//
// Columns: a model's exported struct fields map to columns.
//
//   - the column name is the field's `db` tag, or the field name in
//     snake_case; `db:"-"` skips the field;
//   - fields of embedded structs without a `db` tag are flattened in;
//   - the table must have an "id" text column (the primary key, written from
//     the model's Id());
//   - every table has bigint "created_on" and "updated_on" columns, which Save
//     stamps; embed odm.Timestamps to read them.
//
// Go types map to SQL types:
//
//	string text · bool boolean · int, int64, uint* bigint · int32 integer
//	int16, int8 smallint · float64 double precision · float32 real
//	time.Time timestamp with time zone · []byte bytea
//	slices of those scalars: arrays (text[], bigint[], ...)
//	[]float32 named by a VectorIndexSpec: vector(N) (halfvec(N) for scalar
//	quantization or more than 2000 dimensions)
//	anything else (structs, maps, other slices): jsonb, via encoding/json
//
// Columns are NOT NULL with the type's zero value as default, so adding a
// field to a model adds a column existing rows can be read from. Pointer
// fields and vector columns are nullable.
//
// Tables never change the schema at runtime: EnsureTable creates and migrates
// tables and indexes. Reads of a table that doesn't exist find nothing;
// writes to it fail.
package postgres
