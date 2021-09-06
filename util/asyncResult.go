package util

// Provides a generic struct for functions that return Channel.
// The function can return a generic result and error asynchronously.
type AsyncResult struct {
	Value interface{}
	Err   error
}
