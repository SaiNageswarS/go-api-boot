package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unit tests: no database needed.

// fakeRow scans one value, or fails with err.
type fakeRow struct {
	value any
	err   error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	switch d := dest[0].(type) {
	case *bool:
		*d = r.value.(bool)
	case *string:
		*d = r.value.(string)
	}
	return nil
}

// fakePool answers QueryRow from rows, in order, and fails Exec. Other
// methods panic (the embedded pool is nil).
type fakePool struct {
	pool
	rows []fakeRow
}

func (p *fakePool) QueryRow(context.Context, string, ...any) pgx.Row {
	r := p.rows[0]
	if len(p.rows) > 1 {
		p.rows = p.rows[1:]
	}
	return r
}

func (*fakePool) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New(`extension "pg_search" is not available`)
}

type searchOnly struct {
	ID   string `db:"id"`
	Plot string `db:"plot"`
}

func (s searchOnly) Id() string           { return s.ID }
func (searchOnly) CollectionName() string { return "movies" }
func (searchOnly) TermSearchIndexes() []odm.TermSearchIndexSpec {
	return []odm.TermSearchIndexSpec{{Name: "plotIndex", Paths: []string{"plot"}}}
}

func TestPgSearchRequired(t *testing.T) {
	db := newDB(&fakePool{rows: []fakeRow{{value: false}}})
	err := EnsureTable[searchOnly](t.Context(), db, "tenant")
	assert.ErrorIs(t, err, ErrPgSearchNotInstalled)
	assert.ErrorContains(t, err, "odm/postgres: EnsureTable tenant.movies: term search needs")

	_, err = async.Await(TableOf[searchOnly](db, "tenant").TermSearch(t.Context(), "plotIndex", "crew", 5))
	assert.ErrorIs(t, err, ErrPgSearchNotInstalled)
}

func TestIterativeScansCachesOnlySuccess(t *testing.T) {
	db := newDB(&fakePool{rows: []fakeRow{{err: context.Canceled}, {value: "0.8.0"}}})

	_, err := db.iterativeScans(t.Context())
	assert.ErrorIs(t, err, context.Canceled, "a failed probe is reported")

	ok, err := db.iterativeScans(t.Context())
	require.NoError(t, err)
	assert.True(t, ok, "and retried, not cached as false")

	db.pool = nil // a cached answer needs no query
	ok, err = db.iterativeScans(t.Context())
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestPgvectorAtLeast(t *testing.T) {
	for version, want := range map[string]bool{"0.8.0": true, "0.10.1": true, "1.0": true, "0.7.4": false, "": false} {
		assert.Equal(t, want, pgvectorAtLeast(version, 0, 8), version)
	}
}
