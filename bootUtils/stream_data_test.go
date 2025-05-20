package bootUtils

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

/*
─────────────────────────────────────────────────────────────────────────────

	StreamContextError

─────────────────────────────────────────────────────────────────────────────
*/
func TestStreamContextError(t *testing.T) {
	// 1️⃣ canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := StreamContextError(ctx)
	if status.Code(err) != codes.Canceled {
		t.Fatalf("expected codes.Canceled, got %v", status.Code(err))
	}

	// 2️⃣ deadline exceeded
	ctx, cancel = context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(2 * time.Nanosecond)
	err = StreamContextError(ctx)
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("expected codes.DeadlineExceeded, got %v", status.Code(err))
	}

	// 3️⃣ healthy ctx
	err = StreamContextError(context.Background())
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

/*
─────────────────────────────────────────────────────────────────────────────

	BufferGrpcServerStream

─────────────────────────────────────────────────────────────────────────────
*/
func TestBufferGrpcServerStream_HappyPath(t *testing.T) {
	// a tiny valid PNG header (8 bytes) + filler to reach 512.
	var pkt bytes.Buffer
	pkt.Write([]byte("\x89PNG\r\n\x1a\n"))
	pkt.Write(bytes.Repeat([]byte{0xFF}, 504)) // padding

	read := makeChunkReader(pkt.Bytes(), 128) // returns 4 reads: 128*4 = 512

	buf, ctype, err := BufferGrpcServerStream(
		[]string{"image/png", "application/octet-stream"},
		2048,
		read,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctype != "image/png" {
		t.Fatalf("content-type: got %q, want %q", ctype, "image/png")
	}
	if !bytes.Equal(buf.Bytes(), pkt.Bytes()) {
		t.Fatalf("buffer mismatch")
	}
}

func TestBufferGrpcServerStream_InvalidMime(t *testing.T) {
	// fake GIF header (not in allowed list)
	data := append([]byte("GIF87a"), bytes.Repeat([]byte{0}, 507)...)

	_, _, err := BufferGrpcServerStream(
		[]string{"image/png"},
		2048,
		makeChunkReader(data, 1024),
	)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument for bad mime, got %v", err)
	}
}

func TestBufferGrpcServerStream_SizeLimit(t *testing.T) {
	data := bytes.Repeat([]byte{0xAA}, 1025) // 1 KB + 1

	_, _, err := BufferGrpcServerStream(
		[]string{"application/octet-stream"},
		1024, // limit
		makeChunkReader(data, 512),
	)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument for size limit, got %v", err)
	}
}

// helper: returns a readBytes func that yields data in chunks of n until EOF.
func makeChunkReader(data []byte, n int) func() ([]byte, error) {
	var offset int
	return func() ([]byte, error) {
		if offset >= len(data) {
			return nil, io.EOF
		}
		end := offset + n
		if end > len(data) {
			end = len(data)
		}
		chunk := data[offset:end]
		offset = end
		return chunk, nil
	}
}

/*
─────────────────────────────────────────────────────────────────────────────

	GetFileExtension

─────────────────────────────────────────────────────────────────────────────
*/
func TestGetFileExtension(t *testing.T) {
	tests := map[string]string{
		"image/jpeg": "jpg",
		"video/mp4":  "mp4",
		"unknown":    "",
	}
	for mime, want := range tests {
		if got := GetFileExtension(mime); got != want {
			t.Errorf("mime %q: got %q, want %q", mime, got, want)
		}
	}
}
