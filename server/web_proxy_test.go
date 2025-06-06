package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
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

	assert.Equal(t, 1, len(got), "expected one resource")
	assert.Equal(t, want, got[0], "expected resource name to match")
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
	assert.Equal(t, called, true, "underlying handler should be called")
	assert.Equal(t, http.StatusOK, rec.Code, "expected status 200")
}

func TestReaderCloser_ReadDelegates(t *testing.T) {
	buf := bytes.NewBufferString("hello")
	c := &mockCloser{}

	rc := &readerCloser{reader: buf, closer: c}

	dst := make([]byte, 5)
	n, err := rc.Read(dst)

	// For bytes.Buffer: n == 5 and err == io.EOF (exact read). Either is OK
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", string(dst))
	assert.True(t, err == nil || errors.Is(err, io.EOF))
}

func TestReaderCloser_ReadPropagatesError(t *testing.T) {
	sentinel := errors.New("boom")
	rc := &readerCloser{reader: errReader{sentinel}, closer: io.NopCloser(nil)}

	_, err := rc.Read(make([]byte, 1))
	assert.ErrorIs(t, err, sentinel)
}

func TestReaderCloser_CloseDelegates(t *testing.T) {
	c := &mockCloser{}
	rc := &readerCloser{reader: bytes.NewReader(nil), closer: c}

	err := rc.Close()
	assert.NoError(t, err)
	assert.True(t, c.closed, "underlying closer should be closed")
}

type mockCloser struct {
	closed bool
}

func (m *mockCloser) Close() error {
	m.closed = true
	return nil
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }
