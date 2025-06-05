package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func RetryWithExponentialBackoff(ctx context.Context, maxRetries int, baseDelay time.Duration, fn func() error) error {
	limiter := rate.NewLimiter(rate.Every(baseDelay), 1)
	retries := 0

	for retries < maxRetries {
		// Wait for the next retry attempt
		if err := limiter.Wait(ctx); err != nil {
			return err
		}

		// Attempt the operation
		if err := fn(); err != nil {
			logger.Error("Failed attempt. ", zap.Int("Try", retries+1), zap.Error(err))
			retries++
			// Increase delay exponentially
			limiter.SetLimit(rate.Every(baseDelay * time.Duration(1<<retries)))
		} else {
			logger.Info("Succeeded")
			return nil
		}
	}

	return errors.New("all attempts failed")
}

/*
* Used to buffer a client-side gRPC stream.
* Example usage:
*   rpc UploadProfileImage(stream UploadImageRequest) returns (UploadImageResponse) {}
*
*  	message UploadImageRequest { bytes chunkData = 1; }
*
*  	// server-side handler
*  	func (s *Server) UploadProfileImage(stream grpc.ClientStreamingServer[pb.UploadImageRequest, pb.UploadImageResponse]) error {
* 		ctx := stream.Context()
* 		acceptable := map[string]struct{}{
* 			"image/jpeg": {},
* 			"image/png":  {},
* 		}
*
* 		data, mime, err := BufferGrpcStream(ctx, stream, acceptable, 10*1024*1024) // 10 MB limit
* 	}
 */
func BufferGrpcStream[M DataChunk](
	ctx context.Context,
	stream RecvStream[M], // <-- pointer to the gRPC stream
	acceptable map[string]struct{}, // O(1) lookup
	maxSize int,
) ([]byte, string, error) {

	var head [512]byte
	headFilled := 0

	buf := bytes.Buffer{}
	size := 0

	for {
		if ctx.Err() != nil { // honour deadline / cancel
			return nil, "", status.Error(codes.Canceled, "client canceled upload")
		}

		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", status.Errorf(codes.Internal, "recv failed: %v", err)
		}

		data := chunk.GetData()
		size += len(data)
		if size > maxSize {
			return nil, "", status.Error(codes.InvalidArgument, "file too large")
		}

		if _, err := buf.Write(data); err != nil {
			return nil, "", status.Errorf(codes.Internal, "write buffer: %v", err)
		}

		// capture first 512 B once
		if headFilled < 512 {
			n := copy(head[headFilled:], data)
			headFilled += n
		}
	}

	mime := http.DetectContentType(head[:headFilled])
	if _, ok := acceptable[mime]; !ok {
		return nil, "", status.Error(codes.InvalidArgument, "unacceptable mime type")
	}
	return buf.Bytes(), mime, nil
}

type DataChunk interface {
	GetData() []byte
}

type RecvStream[M DataChunk] interface {
	Recv() (M, error)
}
