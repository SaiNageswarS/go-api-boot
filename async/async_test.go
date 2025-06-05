package async

import (
	"errors"
	"reflect"
	"testing"
)

// helper to quickly build a ready channel
func ready[T any](v T, err error) <-chan Result[T] {
	ch := make(chan Result[T], 1)
	ch <- Result[T]{Data: v, Err: err}
	close(ch)
	return ch
}

func TestAwait(t *testing.T) {
	want := 42
	got, err := Await(ready(want, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("want %d, got %d", want, got)
	}
}

func TestAwaitAllSuccess(t *testing.T) {
	ch1 := ready("a", nil)
	ch2 := ready("b", nil)

	got, err := AwaitAll(ch1, ch2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestAwaitAllPropagatesError(t *testing.T) {
	e := errors.New("boom")
	chGood := ready(1, nil)
	chBad := ready(0, e)

	_, err := AwaitAll(chGood, chBad)
	if !errors.Is(err, e) {
		t.Fatalf("want error %v, got %v", e, err)
	}
}

func TestGoHelper(t *testing.T) {
	ch := Go(func() (int, error) { return 7, nil })
	got, err := Await(ch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 7 {
		t.Fatalf("want 7, got %d", got)
	}
}

func TestAwaitAllWithUnbuffered(t *testing.T) {
	chGood := make(chan Result[int])
	chBad := make(chan Result[int])

	go func() { chGood <- Result[int]{Data: 1}; close(chGood) }()
	go func() { chBad <- Result[int]{Err: errors.New("fail")}; close(chBad) }()

	_, err := AwaitAll(chGood, chBad)
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}
