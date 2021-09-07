package bootUtils

import (
	"bytes"
	"context"
	"io"

	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func contextError(ctx context.Context) error {
	switch ctx.Err() {
	case context.Canceled:
		return status.Error(codes.Canceled, "request is canceled")
	case context.DeadlineExceeded:
		return status.Error(codes.DeadlineExceeded, "deadline is exceeded")
	default:
		return nil
	}
}

func BufferGrpcServerStream(stream grpc.ServerStream, readBytes func() ([]byte, error)) (bytes.Buffer, error) {
	imageData := bytes.Buffer{}

	for {
		err := contextError(stream.Context())
		if err != nil {
			logger.Error("Failed receiving profile image", zap.Error(err))
			return imageData, err
		}

		chunkData, err := readBytes()
		if err == io.EOF {
			break
		}

		_, err = imageData.Write(chunkData)

		if err != nil {
			logger.Error("Failed receiving profile image", zap.Error(err))
			return imageData, status.Errorf(codes.Internal, "Cannot save image to the store: %v", err)
		}
	}

	return imageData, nil
}
