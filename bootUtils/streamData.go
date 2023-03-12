package bootUtils

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
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

// Output: Returns byte buffer of stream and mime-type of the stream.
// Input: acceptableMimeTypes, fileSizeLimit and a stream readByte function.
// To accept any kind of stream pass "application/octet-stream" in acceptableStreams.
func BufferGrpcServerStream(acceptableMimeTypes []string, maxFileSize int, readBytes func() ([]byte, error)) (bytes.Buffer, string, error) {
	imageData := bytes.Buffer{}
	contentType := "application/octet-stream"
	headerChecked := false

	for {
		chunkData, err := readBytes()
		if err == io.EOF {
			break
		}

		_, err = imageData.Write(chunkData)

		if err != nil {
			logger.Error("Failed receiving profile image", zap.Error(err))
			return imageData, contentType, status.Errorf(codes.Internal, "Cannot save image to the store: %v", err)
		}

		// check header only once.
		if !headerChecked && len(imageData.Bytes()) >= 512 {
			contentType = http.DetectContentType(imageData.Bytes())
			isValidContent := slices.Contains(acceptableMimeTypes, contentType)

			if !isValidContent {
				return imageData, contentType, status.Error(codes.InvalidArgument, "Not acceptable mime-type found")
			}

			headerChecked = true
		}

		if len(imageData.Bytes()) > maxFileSize {
			return imageData, contentType, status.Error(codes.InvalidArgument, "Exceeds file size limit.")
		}
	}

	return imageData, contentType, nil
}
