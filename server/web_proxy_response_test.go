package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/net/http2"
)

/*
---------------------------------------------------------------------

	Helpers: a minimal ResponseWriter + Flusher that records everything.
	---------------------------------------------------------------------
*/
type mockRW struct {
	hdr    http.Header
	status int
	buf    bytes.Buffer
}

func newMockRW() *mockRW { return &mockRW{hdr: http.Header{}} }

func (m *mockRW) Header() http.Header         { return m.hdr }
func (m *mockRW) Write(b []byte) (int, error) { return m.buf.Write(b) }
func (m *mockRW) WriteHeader(code int)        { m.status = code }
func (m *mockRW) Flush()                      {} // no-op, but implements http.Flusher

/*
---------------------------------------------------------------------
 1. base64ResponseWriter: write + flush -> encoded output
    ---------------------------------------------------------------------
*/
func TestBase64ResponseWriter_WriteAndFlush(t *testing.T) {
	under := newMockRW()
	w := newBase64ResponseWriter(under)

	_, _ = w.Write([]byte("hello"))
	w.(http.Flusher).Flush() // flush encoded buffer

	got := under.buf.String()
	want := base64.StdEncoding.EncodeToString([]byte("hello"))
	if got != want {
		t.Fatalf("base64 writer: want %q, got %q", want, got)
	}

	// encoder should have been re-created; second write must append
	_, _ = w.Write([]byte("!"))
	w.(http.Flusher).Flush()
	want += base64.StdEncoding.EncodeToString([]byte("!"))
	if under.buf.String() != want {
		t.Fatalf("base64 writer (2nd): want %q, got %q", want, under.buf.String())
	}
}

/*
---------------------------------------------------------------------
 2. prepareHeaders replaces gRPC content-type & exposes headers
    ---------------------------------------------------------------------
*/
func TestWebProxyResponse_PrepareHeaders(t *testing.T) {
	under := newMockRW()
	wp := getWebProxyResponse(under, false)

	// simulate handler headers
	wp.Header().Set("Content-Type", grpcContentType)
	wp.Header().Add("Trailer", "custom-md") // will be skipped
	wp.Header().Set(http2.TrailerPrefix+"grpc-status", "0")

	wp.WriteHeader(http.StatusOK) // triggers prepareHeaders

	if c := under.Header().Get("Content-Type"); c != grpcWebContentType {
		t.Fatalf("Content-Type not replaced: got %q", c)
	}
	exp := []string{"grpc-status", "grpc-message"}
	gotExpose := under.Header().Get("Access-Control-Expose-Headers")

	for _, f := range exp {
		if !strings.Contains(strings.ToLower(gotExpose), f) {
			t.Fatalf("expose headers missing %q: got %q", f, gotExpose)
		}
	}
}

/*
---------------------------------------------------------------------
 3. copyTrailersToPayload writes trailer frame & flushes
    ---------------------------------------------------------------------
*/
func TestWebProxyResponse_CopyTrailers(t *testing.T) {
	under := newMockRW()
	wp := getWebProxyResponse(under, false)

	// Pretend we already wrote body
	body := []byte("dummy") // 5 bytes
	wp.Write(body)

	// Add trailers
	wp.Header().Set(http2.TrailerPrefix+"grpc-status", "0")
	wp.Header().Set(http2.TrailerPrefix+"grpc-message", "OK")

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/", nil)
	wp.finishRequest(req)

	raw := under.buf.Bytes()
	if len(raw) < len(body)+5 {
		t.Fatalf("response too short; got %d bytes", len(raw))
	}

	headerOff := len(body) // trailer frame starts here
	if raw[headerOff]&0x80 == 0 {
		t.Fatalf("missing trailer frame header byte; got 0x%02x", raw[headerOff])
	}

	size := int(binary.BigEndian.Uint32(raw[headerOff+1 : headerOff+5]))
	if size != len(raw)-headerOff-5 {
		t.Fatalf("trailer length mismatch: header=%d, payload=%d",
			size, len(raw)-headerOff-5)
	}

	if !bytes.Contains(raw[headerOff+5:], []byte("grpc-status: 0")) {
		t.Fatalf("trailer payload missing status: %q", raw[headerOff+5:])
	}
}
