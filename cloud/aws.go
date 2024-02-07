package cloud

import (
	"bytes"
	"fmt"
	"time"

	"github.com/SaiNageswarS/go-api-boot/bootUtils"
	"github.com/SaiNageswarS/go-api-boot/logger"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

type AWS struct{}

func (c *AWS) LoadSecretsIntoEnv() {
	err := godotenv.Load()
	if err != nil {
		logger.Error("Error loading .env file", zap.Error(err))
	}

	//TODO: Load secrets from aws secrets manager
}

func (c *AWS) UploadStream(bucketName, path string, imageData bytes.Buffer) (chan string, chan error) {
	//TODO: Upload stream to aws bucket
	return nil, nil
}

// Returns pre-signed upload Url and download URL.
func (c *AWS) GetPresignedUrl(bucketName, path string, expiry time.Duration) (string, string) {
	awsRegion := bootUtils.GetEnv("AWS_REGION", "ap-south-1")

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
