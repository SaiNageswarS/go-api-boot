package server

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestSslCloudCache_Get_NoBucketEnv(t *testing.T) {
	t.Setenv("SSL_BUCKET", "") // unset
	cc := NewSslCloudCache(&mockCloud{})

	_, err := cc.Get(context.Background(), "does-not-matter.pem")
	if err == nil || err.Error() != "SSL_BUCKET environment variable is not set" {
		t.Fatalf("expected env-var error, got %v", err)
	}
}

func TestSslCloudCache_Get_Success(t *testing.T) {
	// Prepare temp file that DownloadFile “hands back”.
	tmp, err := os.CreateTemp(t.TempDir(), "cert-*")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	want := []byte("dummy-certificate-bytes")
	if _, err := tmp.Write(want); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	tmp.Close()

	dataCh := make(chan string, 1)
	errCh := make(chan error, 1)
	dataCh <- tmp.Name()

	mock := &mockCloud{
		downloadDataChan: dataCh,
		downloadErrChan:  errCh,
	}

	t.Setenv("SSL_BUCKET", "my-bucket")
	cc := NewSslCloudCache(mock)

	got, err := cc.Get(context.Background(), "cert.pem")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("bytes mismatch: got %q want %q", got, want)
	}
}

func TestSslCloudCache_Get_DownloadError(t *testing.T) {
	dataCh := make(chan string, 1)
	errCh := make(chan error, 1)
	wantErr := errors.New("boom")
	errCh <- wantErr

	mock := &mockCloud{
		downloadDataChan: dataCh,
		downloadErrChan:  errCh,
	}

	t.Setenv("SSL_BUCKET", "my-bucket")
	cc := NewSslCloudCache(mock)

	_, err := cc.Get(context.Background(), "cert.pem")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

func TestSslCloudCache_Put_NoBucketEnv(t *testing.T) {
	t.Setenv("SSL_BUCKET", "")
	cc := NewSslCloudCache(&mockCloud{})

	err := cc.Put(context.Background(), "cert.pem", []byte("bytes"))
	if err == nil || err.Error() != "SSL_BUCKET environment variable is not set" {
		t.Fatalf("expected env-var error, got %v", err)
	}
}

func TestSslCloudCache_Put_Success(t *testing.T) {
	resCh := make(chan string, 1)
	errCh := make(chan error, 1)
	resCh <- "https://random/file/url" // signal success

	mock := &mockCloud{
		uploadResChan: resCh,
		uploadErrChan: errCh,
	}

	t.Setenv("SSL_BUCKET", "my-bucket")
	cc := NewSslCloudCache(mock)

	if err := cc.Put(context.Background(), "cert.pem", []byte("bytes")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSslCloudCache_Put_Error(t *testing.T) {
	resCh := make(chan string, 1)
	errCh := make(chan error, 1)
	wantErr := errors.New("upload failed")
	errCh <- wantErr

	mock := &mockCloud{
		uploadResChan: resCh,
		uploadErrChan: errCh,
	}

	t.Setenv("SSL_BUCKET", "my-bucket")
	cc := NewSslCloudCache(mock)

	if err := cc.Put(context.Background(), "cert.pem", []byte("bytes")); !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

type mockCloud struct {
	downloadDataChan chan string
	downloadErrChan  chan error
	uploadResChan    chan string
	uploadErrChan    chan error
}

func (m *mockCloud) DownloadFile(bucketName, path string) (chan string, chan error) {
	return m.downloadDataChan, m.downloadErrChan
}

func (m *mockCloud) UploadStream(bucket, name string, data []byte) (chan string, chan error) {
	return m.uploadResChan, m.uploadErrChan
}

func (m *mockCloud) LoadSecretsIntoEnv() {
	// No-op for testing
}

func (m *mockCloud) GetPresignedUrl(bucketName, path, contentType string, expiry time.Duration) (string, string) {
	// No-op for testing
	return "", ""
}
