// Package asyncx runs ODM calls asynchronously, turning panics into errors.
package asyncx

import (
	"fmt"
	"runtime/debug"

	"github.com/SaiNageswarS/go-collection-boot/async"
)

// Go is async.Go, except that a panic in fn (for example the bson encoder
// rejecting a value) is delivered as an error instead of crashing the process.
func Go[T any](fn func() (T, error)) <-chan async.Result[T] {
	return async.Go(func() (v T, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("odm: panic: %v\n%s", r, debug.Stack())
			}
		}()
		return fn()
	})
}
