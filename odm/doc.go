// Package odm holds what go-api-boot's two data backends share: the Model
// contract, timestamps, index declarations, search hits and errors.
//
// Each backend has its own API, native to its database:
//
//   - odm/mongodb: collections queried with MongoDB filter documents (bson.M),
//     with Atlas Search and Vector Search;
//   - odm/postgres: typed tables queried with SQL conditions, with pgvector
//     and ParadeDB search.
//
// Every IO method in both returns a channel carrying an async.Result, so
// independent queries run concurrently:
//
//	movie, err := async.Await(movies.FindByID(ctx, id))
//
// Search indexes are declared as spec values (see VectorIndexSpec) that the
// search methods take, so an index is referred to by a Go identifier, not a
// name string.
//
// Errors from both backends read "odm/<backend>: <tenant>.<name>: <cause>"
// and wrap their cause, so errors.Is matches ErrNotFound, ErrDuplicateKey and
// the driver's own errors.
package odm
