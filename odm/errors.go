package odm

import "errors"

var (
	// ErrNotFound is returned by FindByID, FindOne and First when nothing
	// matches.
	ErrNotFound = errors.New("odm: document not found")

	// ErrDuplicateKey is returned (wrapped) by Save when a unique index rejects
	// the document.
	ErrDuplicateKey = errors.New("odm: duplicate key")
)

// Index features a backend doesn't support return errors wrapping
// errors.ErrUnsupported.
