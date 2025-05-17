package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/cors"
	"google.golang.org/grpc"
)

func TestBuildGrpcServer_ReturnsNonNil(t *testing.T) {
	unaryHit := false
	unary := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (interface{}, error) {
		unaryHit = true
		return h(ctx, req)
	}
	s := buildGrpcServer([]grpc.UnaryServerInterceptor{unary}, nil)
	if s == nil {
		t.Fatalf("buildGrpcServer returned nil")
	}
	// We only care that the server is constructed; exercising interceptors
	// would require registering a service and making a real RPC call.
	if unaryHit {
		t.Fatalf("interceptor ran unexpectedly")
	}
}

func TestAddHandlersToServeMux_WiresHandlers(t *testing.T) {
	mux := http.NewServeMux()
	addHandlersToServeMux(mux, map[string]func(http.ResponseWriter, *http.Request){
		"/test": func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") },
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("handler not wired: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestBuildWebServer_BasicRoutes(t *testing.T) {
	// dummy “gRPC-wrapped” handler
	grpcHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "grpc")
	})

	c := cors.New(cors.Options{AllowOriginFunc: func(string) bool { return true }})
	extra := map[string]func(http.ResponseWriter, *http.Request){
		"/extra": func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "extra") },
	}

	srv := buildWebServer(grpcHandler, c, extra)

	tests := []struct {
		path, wantBody string
		wantCode       int
	}{
		{"/", "grpc", http.StatusOK},
		{"/extra", "extra", http.StatusOK},
		{"/health", "", http.StatusOK},
		{"/metrics", "", http.StatusOK},
	}

	for _, tc := range tests {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		srv.Handler.ServeHTTP(rec, req)

		if rec.Code != tc.wantCode {
			t.Errorf("%s: code=%d want=%d", tc.path, rec.Code, tc.wantCode)
		}
		if tc.wantBody != "" && rec.Body.String() != tc.wantBody {
			t.Errorf("%s: body=%q want=%q", tc.path, rec.Body.String(), tc.wantBody)
		}
	}
}

func TestGetListener_OpensPort(t *testing.T) {
	lis := getListener(":0") // let the OS pick a free port
	if lis == nil {
		t.Fatalf("listener is nil")
	}
	addr := lis.Addr().(*net.TCPAddr)
	if addr.Port == 0 {
		t.Fatalf("listener has invalid port 0")
	}
	lis.Close()
}
