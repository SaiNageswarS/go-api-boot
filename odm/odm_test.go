package odm

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestHashedKey(t *testing.T) {
	a, err := HashedKey("x", "y")
	require.NoError(t, err)
	b, _ := HashedKey("x", "y")
	c, _ := HashedKey("y", "x")
	assert.Equal(t, a, b, "deterministic")
	assert.NotEqual(t, a, c, "order matters")
	assert.Len(t, a, 12)

	empty, err := HashedKey()
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestNewModelFrom(t *testing.T) {
	type src struct{ Name string }
	type dst struct{ Name string }
	assert.Equal(t, "n", NewModelFrom[dst](src{Name: "n"}).Name)
}

func TestTimestampsEmbed(t *testing.T) {
	type profile struct {
		Timestamps `bson:",inline"`
		ID         string `bson:"_id"`
	}
	p := profile{Timestamps: Timestamps{CreatedOn: 1, UpdatedOn: 2}, ID: "a"}

	raw, err := bson.Marshal(p)
	require.NoError(t, err)
	assert.Equal(t, `{"createdOn": {"$numberLong":"1"},"updatedOn": {"$numberLong":"2"},"_id": "a"}`, bson.Raw(raw).String())

	js, err := json.Marshal(p)
	require.NoError(t, err)
	assert.JSONEq(t, `{"createdOn":1,"updatedOn":2,"ID":"a"}`, string(js))
}

func TestSystemTimer(t *testing.T) {
	assert.InDelta(t, time.Now().Unix(), SystemTimer{}.Now(), 1)
}
