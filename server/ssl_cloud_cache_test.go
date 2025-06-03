package server

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/SaiNageswarS/go-api-boot/config"
)

func TestSslCloudCache_Get_NoBucketEnv(t *testing.T) {
	config := &config.BootConfig{}
	cc := NewSslCloudCache(config, &mockCloud{})

	_, err := cc.Get(context.Background(), "does-not-matter.pem")
	if err == nil || err.Error() != "SslBucket not set" {
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

	mock := &mockCloud{
		downloadData: tmp.Name(),
		downloadErr:  nil,
	}

	config := &config.BootConfig{SslBucket: "my-bucket"}
	cc := NewSslCloudCache(config, mock)

	got, err := cc.Get(context.Background(), "cert.pem")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("bytes mismatch: got %q want %q", got, want)
	}
}

func TestSslCloudCache_Get_DownloadError(t *testing.T) {
	wantErr := errors.New("boom")

	mock := &mockCloud{
		downloadData: "",
		downloadErr:  wantErr,
	}

	config := &config.BootConfig{SslBucket: "my-bucket"}
	cc := NewSslCloudCache(config, mock)

	_, err := cc.Get(context.Background(), "cert.pem")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

func TestSslCloudCache_Put_NoBucketEnv(t *testing.T) {
	config := &config.BootConfig{}
	cc := NewSslCloudCache(config, &mockCloud{})

	err := cc.Put(context.Background(), "cert.pem", []byte("bytes"))
	if err == nil || err.Error() != "SslBucket not set" {
		t.Fatalf("expected env-var error, got %v", err)
	}
}

func TestSslCloudCache_Put_Success(t *testing.T) {
	mock := &mockCloud{
		uploadRes: "https://random/file/url",
		uploadErr: nil,
	}

	config := &config.BootConfig{SslBucket: "my-bucket"}
	cc := NewSslCloudCache(config, mock)

	if err := cc.Put(context.Background(), "cert.pem", []byte("bytes")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSslCloudCache_Put_Error(t *testing.T) {
	wantErr := errors.New("upload failed")

	mock := &mockCloud{
		uploadRes: "",
		uploadErr: wantErr,
	}

	config := &config.BootConfig{SslBucket: "my-bucket"}
	cc := NewSslCloudCache(config, mock)

	if err := cc.Put(context.Background(), "cert.pem", []byte("bytes")); !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

type mockCloud struct {
	downloadData string
	downloadErr  error
	uploadRes    string
	uploadErr    error
}

func (m *mockCloud) DownloadFile(ctx context.Context, bucketName, path string) (string, error) {
	return m.downloadData, m.downloadErr
}

func (m *mockCloud) UploadBuffer(ctx context.Context, bucket, name string, data []byte) (string, error) {
	return m.uploadRes, m.uploadErr
}

func (m *mockCloud) LoadSecretsIntoEnv(ctx context.Context) {
	// No-op for testing
}

func (m *mockCloud) GetPresignedUrl(ctx context.Context, bucketName, path, contentType string, expiry time.Duration) (string, string) {
	// No-op for testing
	return "", ""
}
