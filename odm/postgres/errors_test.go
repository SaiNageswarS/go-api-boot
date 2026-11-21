package postgres

import (
	"testing"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/stretchr/testify/assert"
)

// Unit tests: no database needed.

func TestAnnotate(t *testing.T) {
	var err error
	annotate(&err, "tenant", "movies")
	assert.NoError(t, err)

	err = odm.ErrNotFound
	annotate(&err, "tenant", "movies")
	assert.EqualError(t, err, "odm/postgres: tenant.movies: document not found")
	assert.ErrorIs(t, err, odm.ErrNotFound)
}
