package cloud

import (
	"context"
	"fmt"
	"time"

	"github.com/SaiNageswarS/go-api-boot/bootUtils"
	"github.com/SaiNageswarS/go-api-boot/config"
	"github.com/SaiNageswarS/go-api-boot/logger"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"go.uber.org/zap"
)

type AWS struct {
	ccfgg *config.BootConfig
}

// ProvideAWS returns an AWS cloud client.
func ProvideAWS(ccfgg *config.BootConfig) Cloud {
	return &AWS{
		ccfgg: ccfgg,
	}
}

func (c *AWS) LoadSecretsIntoEnv(ctx context.Context) {
	//TODO: Load secrets from aws secrets manager
}

func (c *AWS) UploadStream(ctx context.Context, bucketName, path string, fileData []byte) (string, error) {
	//TODO: Upload stream to aws bucket
	return "", nil
}

func (c *AWS) DownloadFile(ctx context.Context, bucketName, path string) (string, error) {
	//TODO: Download file from aws bucket
	return "", nil
}

// Returns pre-signed upload Url and download URL.
func (c *AWS) GetPresignedUrl(ctx context.Context, bucketName, path, contentType string, expiry time.Duration) (string, string) {
	awsRegion := bootUtils.GetEnvOrDefault("AWS_REGION", "ap-south-1")

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion)},
	)
	if err != nil {
		logger.Error("Error getting aws session", zap.Error(err))
		return "", ""
	}

	// Create S3 service client
	svc := s3.New(sess)
	req, _ := svc.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(path),
	})

	urlStr, err := req.Presign(expiry)
	if err != nil {
		logger.Error("Error signing s3 url", zap.Error(err))
		return "", ""
	}

	downloadUrl := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", bucketName, awsRegion, path)
	return urlStr, downloadUrl
}
