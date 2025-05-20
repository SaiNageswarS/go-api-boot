package server

import (
	"context"
	"reflect"
	"testing"

	"github.com/SaiNageswarS/go-api-boot/config"
	"google.golang.org/grpc"
)

/* ───────────────────────── helpers used in tests ─────────────────────────── */

// Simple dependency + service types
type dep struct{ id int }
type svc struct{ d *dep }

// register spy – captures arguments passed by Builder.Register
type regSpy struct {
	called  int
	gotSrv  any
	gotGRPC grpc.ServiceRegistrar
}

func (r *regSpy) fn(g grpc.ServiceRegistrar, s any) {
	r.called++
	r.gotGRPC = g
	r.gotSrv = s
}

// singleton dep provider (used in memoisation test)
type depProvider struct{ called int }

func (p *depProvider) provide() *dep { p.called++; return &dep{id: 7} }

/* ───────────────────────────────  TESTS  ─────────────────────────────────── */

func TestBuilder_BuildValidation(t *testing.T) {
	_, err := New(&config.BootConfig{}).
		GRPCPort(":0").
		Build() // missing HTTPPort
	if err == nil {
		t.Fatalf("Build() succeeded with missing HTTPPort; want error")
	}

	_, err = New(&config.BootConfig{}).
		HTTPPort(":0").
		Build() // missing GRPCPort
	if err == nil {
		t.Fatalf("Build() succeeded with missing GRPCPort; want error")
	}
}

func TestBuilder_Provide_RegistersService(t *testing.T) {
	// arrange
	d := &dep{id: 42}
	spy := &regSpy{}

	builder := New(&config.BootConfig{}).
		GRPCPort(":0").
		HTTPPort(":0").
		Provide(d).
		Register(spy.fn, func(dd *dep) *svc {
			return &svc{d: dd}
		})

	// act
	if _, err := builder.Build(); err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	// assert register callback executed exactly once
	if spy.called != 1 {
		t.Fatalf("register fn called %d times, want 1", spy.called)
	}
	s, ok := spy.gotSrv.(*svc)
	if !ok {
		t.Fatalf("register fn received wrong service type: %T", spy.gotSrv)
	}
	if s.d != d {
		t.Fatalf("dependency injection failed: got %p, want %p", s.d, d)
	}
	if spy.gotGRPC == nil {
		t.Fatalf("grpc.Server pointer was nil")
	}
}

func TestBuilder_ProvideFunc_Memoised(t *testing.T) {
	// arrange provider that records call count
	p := &depProvider{}
	spy1, spy2 := &regSpy{}, &regSpy{}

	b := New(&config.BootConfig{}).
		GRPCPort(":0").
		HTTPPort(":0").
		ProvideFunc(p.provide).
		Register(spy1.fn, func(d *dep) *svc { return &svc{d: d} }).
		Register(spy2.fn, func(d *dep) *svc { return &svc{d: d} })

	// act
	if _, err := b.Build(); err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	// assert
	if p.called != 1 {
		t.Fatalf("provider should be memoised; called %d times, want 1", p.called)
	}
	if spy1.called != 1 || spy2.called != 1 {
		t.Fatalf("each register fn must be called once (got %d / %d)",
			spy1.called, spy2.called)
	}
	// both services must share the same *dep instance
	d1 := spy1.gotSrv.(*svc).d
	d2 := spy2.gotSrv.(*svc).d
	if d1 != d2 {
		t.Fatalf("memoisation failed: services received different dep pointers")
	}
}

/* ─────────────────────── utility: ensure compile-time types ──────────────── */

func TestContainerResolveSignature(t *testing.T) {
	// Make an empty container—not nil—so the call is safe.
	c := newContainer(
		make(map[reflect.Type]reflect.Value),
		make(map[reflect.Type]reflect.Value),
	)

	v, err := c.resolve(reflect.TypeOf((*context.Context)(nil)).Elem())
	if err == nil {
		t.Fatalf("expected error for missing provider, got nil (v = %v)", v)
	}
}

type iface interface {
	GetID() int
}

// concrete type implementing iface
type ifaceImpl struct{ id int }

func (i *ifaceImpl) GetID() int { return i.id }

func TestBuilder_ProvideAs_BindsInterface(t *testing.T) {
	impl := &ifaceImpl{id: 99}
	spy := &regSpy{}

	builder := New(&config.BootConfig{}).
		GRPCPort(":0").
		HTTPPort(":0").
		ProvideAs(impl, (*iface)(nil)). // <- use ProvideAs here
		Register(spy.fn, func(i iface) *svc {
			return &svc{d: &dep{id: i.GetID()}} // embed id into dep for validation
		})

	if _, err := builder.Build(); err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	s, ok := spy.gotSrv.(*svc)
	if !ok {
		t.Fatalf("register fn received wrong service type: %T", spy.gotSrv)
	}
	if s.d.id != 99 {
		t.Fatalf("interface method injection failed: got %d, want 99", s.d.id)
	}
}
