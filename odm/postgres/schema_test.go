package postgres

import (
	"testing"
	"time"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unit tests: no database needed.

type address struct {
	City string `json:"city"`
}

type base struct {
	ID        string `db:"id"`
	CreatedOn int64  `db:"created_on"`
}

type profile struct {
	base
	Name      string
	UserID    string
	Score     float64
	Rank      int32
	Tags      []string
	Address   address
	Previous  *address
	Nickname  *string
	Joined    time.Time
	Avatar    []byte
	Embedding []float32 `db:"embedding"`
	Secret    string    `db:"-"`
	hidden    string
}

func (p profile) Id() string             { return p.ID }
func (p profile) CollectionName() string { return "profiles" }

var profileEmbedding = odm.VectorIndexSpec{Name: "byEmbedding", Path: "embedding", Dimensions: 3}

func (p profile) VectorIndexes() []odm.VectorIndexSpec {
	return []odm.VectorIndexSpec{profileEmbedding}
}

func TestSchemaOf(t *testing.T) {
	_ = profile{}.hidden
	s, err := schemaOf[profile]()
	require.NoError(t, err)

	got := map[string]string{}
	var order []string
	for _, c := range s.columns {
		got[c.name] = c.definition()
		order = append(order, c.name)
	}
	assert.Equal(t, []string{"id", "created_on", "name", "user_id", "score", "rank", "tags", "address",
		"previous", "nickname", "joined", "avatar", "embedding", "updated_on"}, order,
		"embedded fields flatten; db:\"-\" and unexported skip; the unmapped timestamp comes last")
	assert.Equal(t, map[string]string{
		"id":         `"id" text PRIMARY KEY`,
		"created_on": `"created_on" bigint NOT NULL DEFAULT 0`,
		"name":       `"name" text NOT NULL DEFAULT ''`,
		"user_id":    `"user_id" text NOT NULL DEFAULT ''`,
		"score":      `"score" double precision NOT NULL DEFAULT 0`,
		"rank":       `"rank" integer NOT NULL DEFAULT 0`,
		"tags":       `"tags" text[] NOT NULL DEFAULT '{}'`,
		"address":    `"address" jsonb NOT NULL DEFAULT 'null'`,
		"previous":   `"previous" jsonb NOT NULL DEFAULT 'null'`,
		"nickname":   `"nickname" text`,
		"joined":     `"joined" timestamp with time zone NOT NULL DEFAULT '0001-01-01 00:00:00+00'`,
		"avatar":     `"avatar" bytea NOT NULL DEFAULT '\x'`,
		"embedding":  `"embedding" vector(3)`,
		"updated_on": `"updated_on" bigint NOT NULL DEFAULT 0`,
	}, got)
	assert.Len(t, s.fields(), len(s.columns)-1, "updated_on has no field to read into")
	assert.NotContains(t, s.selectList(), "updated_on")
}

func TestSnakeCase(t *testing.T) {
	for in, out := range map[string]string{"Name": "name", "UserID": "user_id", "HTTPServer": "http_server", "ReleasedOn": "released_on", "ID": "id"} {
		assert.Equal(t, out, snakeCase(in))
	}
}

type noID struct {
	Key string `db:"key"`
}

func (noID) Id() string             { return "" }
func (noID) CollectionName() string { return "x" }

type twoNames struct {
	ID   string `db:"id"`
	A    string `db:"name"`
	Name string
}

func (twoNames) Id() string             { return "" }
func (twoNames) CollectionName() string { return "x" }

type badVector struct {
	ID string `db:"id"`
	E  []float64
}

func (badVector) Id() string             { return "" }
func (badVector) CollectionName() string { return "x" }
func (badVector) VectorIndexes() []odm.VectorIndexSpec {
	return []odm.VectorIndexSpec{{Name: "v", Path: "e", Dimensions: 3}}
}

type stringStamp struct {
	ID        string `db:"id"`
	CreatedOn string `db:"created_on"`
}

func (stringStamp) Id() string             { return "" }
func (stringStamp) CollectionName() string { return "x" }

func TestSchemaErrors(t *testing.T) {
	_, err := schemaOf[stringStamp]()
	assert.ErrorContains(t, err, `column "created_on" is stamped by Save and must be an int64 field`)
	_, err = schemaOf[noID]()
	assert.ErrorContains(t, err, `"id" column`)
	_, err = schemaOf[twoNames]()
	assert.ErrorContains(t, err, `two fields to column "name"`)
	_, err = schemaOf[badVector]()
	assert.ErrorContains(t, err, "needs a []float32 field")
}
