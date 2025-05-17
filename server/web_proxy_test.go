package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc"
)

func TestListGRPCResources(t *testing.T) {
	gsrv := grpc.NewServer()

	// Register a minimal service with one unary method.
	svc := grpc.ServiceDesc{
		ServiceName: "test.TestService",
		HandlerType: (*interface{})(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "Ping",
				Handler: func(srv interface{},
					ctx context.Context,
					dec func(interface{}) error,
					interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
					return nil, nil
				},
			},
		},
	}
	gsrv.RegisterService(&svc, struct{}{})

	got := listGRPCResources(gsrv)
	want := "/test.TestService/Ping"

	if len(got) != 1 || got[0] != want {
		t.Fatalf("expected %q, got %#v", want, got)
	}
}

func TestWebProxyServeHTTP(t *testing.T) {
	var called bool

	// A spy handler to verify the request reaches the underlying handler.
	spy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/test" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	})

	// Directly construct WebProxy with the spy handler.
	wp := WebProxy{
		handler:       spy,
		endPointsFunc: func() []string { return []string{"/test"} },
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Content-Type", grpcWebContentType)
	rec := httptest.NewRecorder()

	wp.ServeHTTP(rec, req)

	if !called {
		t.Fatal("underlying handler was not invoked")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}
