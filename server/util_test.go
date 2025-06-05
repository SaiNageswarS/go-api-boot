package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRetryWithExponentialBackoff_SucceedsAfterRetries(t *testing.T) {
	var tries int
	fn := func() error {
		tries++
		if tries < 3 {
			return errors.New("boom")
		}
		return nil
	}

	err := RetryWithExponentialBackoff(
		context.Background(),
		5,
		1*time.Millisecond, // fast test
		fn,
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if tries != 3 {
		t.Fatalf("expected 3 attempts, got %d", tries)
	}
}

func TestRetryWithExponentialBackoff_ExhaustsAndFails(t *testing.T) {
	var tries int
	fn := func() error { tries++; return errors.New("always") }

	err := RetryWithExponentialBackoff(
		context.Background(),
		4,
		1*time.Millisecond,
		fn,
	)
	if err == nil || err.Error() != "all attempts failed" {
		t.Fatalf("expected final failure, got %v", err)
	}
	if tries != 4 {
		t.Fatalf("expected 4 attempts, got %d", tries)
	}
}

func TestRetryWithExponentialBackoff_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	fn := func() error { return errors.New("never called") }

	start := time.Now()
	err := RetryWithExponentialBackoff(ctx, 10, 10*time.Millisecond, fn)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if time.Since(start) > 20*time.Millisecond {
		t.Fatalf("function did not return promptly after cancellation")
	}
}

type mockChunk struct{ b []byte }

func (m mockChunk) GetData() []byte { return m.b }

type mockStream struct {
	chunks []mockChunk
	errAt  int // index that should return a recv error, -1 = never
	err    error
	i      int
}

func (s *mockStream) Recv() (mockChunk, error) {
	// only fire the error if *both* an index and a non-nil error were supplied
	if s.err != nil && s.errAt >= 0 && s.i == s.errAt {
		return mockChunk{}, s.err
	}
	if s.i >= len(s.chunks) {
		return mockChunk{}, io.EOF
	}
	c := s.chunks[s.i]
	s.i++
	return c, nil
}

// helper
func run(t *testing.T, ctx context.Context, st *mockStream, max int) ([]byte, string, error) {
	t.Helper()
	accept := map[string]struct{}{"application/octet-stream": {}}
	return BufferGrpcStream(ctx, st, accept, max)
}

// ─── tests ──────────────────────────────────────────────────────────────
func TestBufferGrpcStream_OK(t *testing.T) {
	data1 := bytes.Repeat([]byte{1}, 300)
	data2 := bytes.Repeat([]byte{2}, 150)

	st := &mockStream{chunks: []mockChunk{{data1}, {data2}}}
	got, mime, err := run(t, context.Background(), st, 1<<20) // 1 MiB limit
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := append(data1, data2...)
	if !bytes.Equal(got, want) {
		t.Fatalf("data mismatch: want %d bytes, got %d", len(want), len(got))
	}
	if mime != "application/octet-stream" {
		t.Fatalf("mime: want application/octet-stream, got %s", mime)
	}
}

func TestBufferGrpcStream_TooLarge(t *testing.T) {
	st := &mockStream{chunks: []mockChunk{{bytes.Repeat([]byte{1}, 1024)}}}
	_, _, err := run(t, context.Background(), st, 500) // 500 B limit
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument, got %v", err)
	}
}

func TestBufferGrpcStream_UnacceptableMime(t *testing.T) {
	st := &mockStream{chunks: []mockChunk{{[]byte("GIF89a")}}} // Detects as image/gif
	accept := map[string]struct{}{"image/png": {}}
	_, _, err := BufferGrpcStream(context.Background(), st, accept, 1024)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument (mime), got %v", err)
	}
}

func TestBufferGrpcStream_RecvError(t *testing.T) {
	st := &mockStream{
		chunks: []mockChunk{{bytes.Repeat([]byte{1}, 10)}},
		errAt:  1,
		err:    errors.New("network"),
	}
	_, _, err := run(t, context.Background(), st, 1024)
	if status.Code(err) != codes.Internal {
		t.Fatalf("want Internal, got %v", err)
	}
}

func TestBufferGrpcStream_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	st := &mockStream{
		chunks: []mockChunk{{bytes.Repeat([]byte{1}, 10)}},
	}
	cancel() // cancel before first Recv
	_, _, err := run(t, ctx, st, 1024)
	if status.Code(err) != codes.Canceled {
		t.Fatalf("want Canceled, got %v", err)
	}
}
