package server

import (
	"errors"
	"reflect"
	"testing"
)

// ─── simple types used in tests ───────────────────────────────────────────────
type A struct{ id int }
type B struct{ dep *A }
type C struct{}

// ──────────────────────────────────────────────────────────────────────────────
// helper: convenience creator
// ──────────────────────────────────────────────────────────────────────────────
func makeContainer() *container {
	return newContainer(make(map[reflect.Type]reflect.Value),
		make(map[reflect.Type]reflect.Value))
}

// ──────────────────────────────────────────────────────────────────────────────
// 1. singleton resolution
// ──────────────────────────────────────────────────────────────────────────────
func TestContainer_ResolveSingleton(t *testing.T) {
	c := makeContainer()

	orig := &A{id: 42}
	c.singletons[reflect.TypeOf(orig)] = reflect.ValueOf(orig)

	v, err := c.resolve(reflect.TypeOf(orig))
	if err != nil {
		t.Fatalf("resolve returned error: %v", err)
	}
	got := v.Interface().(*A)

	if got != orig {
		t.Fatalf("resolve returned %p, want %p (same singleton)", got, orig)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// 2. provider without deps
// ──────────────────────────────────────────────────────────────────────────────
func TestContainer_ResolveProvider(t *testing.T) {
	c := makeContainer()

	c.providers[reflect.TypeOf(&A{})] = reflect.ValueOf(func() *A {
		return &A{id: 7}
	})

	v, err := c.resolve(reflect.TypeOf(&A{}))
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	got := v.Interface().(*A)
	if got.id != 7 {
		t.Fatalf("unexpected value: %+v", got)
	}

	// provider result must be memoised → second resolve same pointer
	v2, _ := c.resolve(reflect.TypeOf(&A{}))
	if got != v2.Interface().(*A) {
		t.Fatalf("provider result not cached; got two distinct pointers")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// 3. provider with nested dependency
// ──────────────────────────────────────────────────────────────────────────────
func TestContainer_ResolveNestedProvider(t *testing.T) {
	c := makeContainer()

	// provider for *A
	c.providers[reflect.TypeOf(&A{})] = reflect.ValueOf(func() *A {
		return &A{id: 11}
	})
	// provider for *B depends on *A
	c.providers[reflect.TypeOf(&B{})] = reflect.ValueOf(func(a *A) *B {
		return &B{dep: a}
	})

	v, err := c.resolve(reflect.TypeOf(&B{}))
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	b := v.Interface().(*B)

	if b.dep == nil || b.dep.id != 11 {
		t.Fatalf("nested dependency not injected: %+v", b)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// 4. missing provider / singleton should error
// ──────────────────────────────────────────────────────────────────────────────
func TestContainer_ResolveMissing(t *testing.T) {
	c := makeContainer()

	_, err := c.resolve(reflect.TypeOf(&C{}))
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, err) { // any non-nil error is fine
		t.Fatalf("unexpected error: %v", err)
	}
}
