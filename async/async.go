package async

type Result[T any] struct {
	Data T
	Err  error
}

func Await[T any](ch <-chan Result[T]) (T, error) {
	res := <-ch
	return res.Data, res.Err
}

func AwaitAll[T any](chs ...<-chan Result[T]) ([]T, error) {
	type pair struct {
		i int
		r Result[T]
	}

	out := make(chan pair, len(chs))
	for i, ch := range chs {
		i, ch := i, ch // capture copy
		go func() { out <- pair{i, <-ch} }()
	}

	results := make([]T, len(chs))
	var firstErr error
	for range chs {
		p := <-out // drains every channel
		if firstErr == nil {
			firstErr = p.r.Err // remember first error
		}
		results[p.i] = p.r.Data
	}
	return results, firstErr
}

func Go[T any](fn func() (T, error)) <-chan Result[T] {
	ch := make(chan Result[T], 1) // buffered so sender never blocks
	go func() {
		defer close(ch)
		v, err := fn()
		ch <- Result[T]{Data: v, Err: err}
	}()
	return ch
}
