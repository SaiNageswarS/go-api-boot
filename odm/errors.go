package odm

import "errors"

var (
	// ErrNotFound is returned by FindByID and FindOne when nothing matches.
	ErrNotFound = errors.New("odm: document not found")

	// ErrDuplicateKey is returned (wrapped) by Save when a unique index rejects
	// the document.
	ErrDuplicateKey = errors.New("odm: duplicate key")
)

// Filters, sorts or index features a backend can't translate return errors
// wrapping errors.ErrUnsupported.
