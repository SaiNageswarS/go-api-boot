package server

import (
	"context"
	"reflect"
	"testing"

	"github.com/rs/cors"
	"github.com/stretchr/testify/assert"
	"go.temporal.io/sdk/client"
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
	_, err := New().
		GRPCPort(":0").
		Build() // missing HTTPPort
	if err == nil {
		t.Fatalf("Build() succeeded with missing HTTPPort; want error")
	}

	_, err = New().
		HTTPPort(":0").
		Build() // missing GRPCPort
	if err == nil {
		t.Fatalf("Build() succeeded with missing GRPCPort; want error")
	}
}

func TestBuilder_RegisterValidation(t *testing.T) {
	mockUnaryInterceptor := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		return handler(ctx, req) // just pass through
	}

	mockStreamInterceptor := func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, ss) // just pass through
	}

	corsConfig := cors.New(
		cors.Options{
			AllowedHeaders: []string{"*"},
		})

	builder := New().
		Unary(mockUnaryInterceptor).
		Stream(mockStreamInterceptor).
		CORS(corsConfig)

	assert.Equal(t, len(builder.unary), 4)  // 3 default + 1 custom
	assert.Equal(t, len(builder.stream), 4) // 3 default + 1 custom
	assert.NotNil(t, builder.cors)
}

func TestBuilder_Provide_RegistersService(t *testing.T) {
	// arrange
	d := &dep{id: 42}
	spy := &regSpy{}

	builder := New().
		GRPCPort(":0").
		HTTPPort(":0").
		Provide(d).
		RegisterService(spy.fn, func(dd *dep) *svc {
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

	b := New().
		GRPCPort(":0").
		HTTPPort(":0").
		ProvideFunc(p.provide).
		RegisterService(spy1.fn, func(d *dep) *svc { return &svc{d: d} }).
		RegisterService(spy2.fn, func(d *dep) *svc { return &svc{d: d} })

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

	builder := New().
		GRPCPort(":0").
		HTTPPort(":0").
		ProvideAs(impl, (*iface)(nil)). // <- use ProvideAs here
		RegisterService(spy.fn, func(i iface) *svc {
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

// ------ Temporal worker tests (if applicable) ------

func TestBuilder_WithTemporal_StoresConfig(t *testing.T) {
	opts := &client.Options{HostPort: "test:7233"}
	b := New().WithTemporal("my-queue", opts)

	if b.taskQueue != "my-queue" {
		t.Errorf("expected taskQueue 'my-queue', got %q", b.taskQueue)
	}
	if b.temporalClientOpts != opts {
		t.Error("expected temporalClientOpts to be stored")
	}
}

func TestRegisterTemporalWorkflow_Appends(t *testing.T) {
	b := New()

	// 1 → slice empty
	if ln := len(b.workflowRegs); ln != 0 {
		t.Fatalf("expected 0 workflows initially, got %d", ln)
	}

	b.RegisterTemporalWorkflow(workflow)

	// 2 → slice grew
	if ln := len(b.workflowRegs); ln != 1 {
		t.Fatalf("expected 1 workflow after registration, got %d", ln)
	}
	if reflect.ValueOf(b.workflowRegs[0]).Pointer() != reflect.ValueOf(workflow).Pointer() {
		t.Errorf("workflow not stored correctly")
	}
}

func TestRegisterTemporalActivity_Appends(t *testing.T) {
	b := New()

	// 1 → empty
	if ln := len(b.activityRegs); ln != 0 {
		t.Fatalf("expected 0 activities initially, got %d", ln)
	}

	b.RegisterTemporalActivity(activityFactory)

	// 2 → grew & contains reflect.Value of factory
	if ln := len(b.activityRegs); ln != 1 {
		t.Fatalf("expected 1 activity after registration, got %d", ln)
	}
	if b.activityRegs[0] != reflect.ValueOf(activityFactory) {
		t.Errorf("activity factory not stored correctly")
	}
}

func TestBuild_WithoutTemporal_Succeeds(t *testing.T) {
	// allocate random high ports so tests can run in parallel; ":0" lets OS choose.
	b := New().GRPCPort(":0").HTTPPort(":0")

	srv, err := b.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if srv.temporalWorker != nil {
		t.Errorf("expected no temporal worker when opts unset")
	}
}

func activityFactory() *struct{} { return &struct{}{} }

func workflow() error { return nil }
