package aws

import (
	"fmt"
	"time"

	"github.com/SaiNageswarS/go-api-boot/logger"
	"github.com/SaiNageswarS/go-api-boot/util"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"go.uber.org/zap"
)

var S3 s3Wrapper = s3Wrapper{}

type s3Wrapper struct {
}

// Returns pre-signed upload Url and download URL.
func (s s3Wrapper) GetPresignedUrl(bucket, key string) (string, string) {
	awsRegion := util.GetEnv("AWS_REGION", "ap-south-1")

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
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})

	urlStr, err := req.Presign(15 * time.Minute)
	if err != nil {
		logger.Error("Error signing s3 url", zap.Error(err))
		return "", ""
	}

	downloadUrl := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", bucket, awsRegion, key)
	return urlStr, downloadUrl
}
