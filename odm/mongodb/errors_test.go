package mongodb

import (
	"testing"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Unit tests: no database needed.

func TestAnnotate(t *testing.T) {
	var err error
	annotate(&err, "tenant.movies")
	assert.NoError(t, err)

	err = mongo.ErrNoDocuments
	annotate(&err, "tenant.movies")
	assert.EqualError(t, err, "odm/mongodb: tenant.movies: document not found")
	assert.ErrorIs(t, err, odm.ErrNotFound)
}
