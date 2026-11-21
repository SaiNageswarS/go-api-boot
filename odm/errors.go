package odm

import "errors"

var (
	// ErrNotFound is returned (wrapped) by the backends' single-document
	// reads, such as FindByID, when nothing matches.
	ErrNotFound = errors.New("document not found")

	// ErrDuplicateKey is returned (wrapped) by Save when a unique index rejects
	// the document.
	ErrDuplicateKey = errors.New("duplicate key")
)
