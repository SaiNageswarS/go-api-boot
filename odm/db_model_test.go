package odm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewModelFrom_Success(t *testing.T) {
	type Proto struct {
		Question    string
		Options     []string
		Answer      string
		Explanation string
	}

	proto := &Proto{
		Question:    "What is the capital of France?",
		Options:     []string{"Paris", "London", "Berlin"},
		Answer:      "Paris",
		Explanation: "Paris is the capital city of France."}

	model := NewModelFrom[mockModel](proto)
	assert.NotNil(t, model)
	assert.Equal(t, "What is the capital of France?", model.Question)
	assert.Equal(t, []string{"Paris", "London", "Berlin"}, model.Options)
	assert.Equal(t, "Paris", model.Answer)
	assert.Equal(t, "Paris is the capital city of France.", model.Explanation)
	assert.Equal(t, "", model.QHash)   // QHash should be empty as it's not set in proto
	assert.Equal(t, "", model.Subject) // Subject should be empty as it's not set in proto
}

type mockModel struct {
	QHash       string   `bson:"qhash"` // hash of question
	Question    string   `bson:"question"`
	Options     []string `bson:"options"`
	Answer      string   `bson:"answer"`
	Explanation string   `bson:"explanation"`
	Subject     string   `bson:"subject"`
	Topic       string   `bson:"topic"`
	Difficulty  string   `bson:"difficulty"`
	CreatedBy   string   `bson:"createdBy"`
}

func (m *mockModel) Id() string {
	if len(m.QHash) == 0 {
		m.QHash, _ = HashedKey(m.Question)
	}

	return m.QHash
}

func (m *mockModel) CollectionName() string {
	return "mock_collection"
}

func TestDefaultTimer(t *testing.T) {
	timer := DefaultTimer{}
	assert.NotNil(t, timer)
	assert.Equal(t, timer.Now(), time.Now().Unix())
}

func TestHashedKey_Deterministic(t *testing.T) {
	h1, err := HashedKey("user", "123", "2025")
	assert.NoError(t, err)

	h2, err := HashedKey("user", "123", "2025")
	assert.NoError(t, err)

	assert.Equal(t, h1, h2, "HashedKey should be deterministic")
}

func TestHashedKey_OrderMatters(t *testing.T) {
	h1, _ := HashedKey("A", "B")
	h2, _ := HashedKey("B", "A")

	assert.NotEqual(t, h1, h2, "Order of fields should affect the hash")
}

func TestHashedKey_DifferentInputsYieldDifferentHashes(t *testing.T) {
	h1, _ := HashedKey("foo", "bar")
	h2, _ := HashedKey("foo", "baz")

	assert.NotEqual(t, h1, h2, "Different inputs should yield different hashes")
}

func TestHashedKey_HashLength(t *testing.T) {
	h, err := HashedKey("abc", "123")
	assert.NoError(t, err)
	assert.Len(t, h, 12, "Hash output should be 12 hex characters long")
}

func TestHashedKey_EmptyInput(t *testing.T) {
	h, err := HashedKey()
	assert.NoError(t, err)
	assert.Len(t, h, 0, "Empty input should yield an empty hash string")
}
