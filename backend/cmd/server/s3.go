package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Client contains the initialized AWS S3 client and the configured bucket.
// Credentials are resolved by the AWS SDK default provider chain, which uses
// the EC2 instance profile when the service runs on EC2.
type S3Client struct {
	Bucket          string
	Client          *s3.Client
	Presigner       *s3.PresignClient
	PresignedExpiry time.Duration
}

const (
	presignedURLExpiryEnv     = "S3_PRESIGNED_URL_EXPIRY_SECONDS"
	defaultPresignedURLExpiry = 15 * time.Minute
	maxPresignedURLExpiry     = 7 * 24 * time.Hour
)

// S3ObjectStore is the S3 operation needed while creating a file-processing
// job. Keeping this small makes request validation unit-testable without AWS.
type S3ObjectStore interface {
	HeadInputObject(context.Context, string) error
}

// S3InputDownloader exposes the streaming operation workers need. The body is
// owned by the caller and must be closed after it has been copied.
type S3InputDownloader interface {
	DownloadInputObject(context.Context, string) (io.ReadCloser, error)
}

type S3OutputUploader interface {
	UploadOutputObject(context.Context, string, io.Reader, string) error
}

type S3OutputPresigner interface {
	PresignOutputGet(context.Context, string) (string, time.Duration, error)
}

type S3Service interface {
	S3ObjectStore
	ListInputFiles(context.Context) ([]S3File, error)
}

type S3File struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// NewS3Client loads the required S3 configuration and initializes an S3
// client. It does not make a network request; AWS operations happen in later
// stages when the client is used.
func NewS3Client(ctx context.Context) (*S3Client, error) {
	bucket := strings.TrimSpace(os.Getenv("S3_BUCKET"))
	if bucket == "" {
		return nil, fmt.Errorf("S3_BUCKET is not set")
	}

	region := strings.TrimSpace(os.Getenv("AWS_REGION"))
	if region == "" {
		return nil, fmt.Errorf("AWS_REGION is not set")
	}

	awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}

	client := s3.NewFromConfig(awsConfig)
	expiry, err := configuredPresignedURLExpiry()
	if err != nil {
		return nil, err
	}
	return &S3Client{
		Bucket:          bucket,
		Client:          client,
		Presigner:       s3.NewPresignClient(client),
		PresignedExpiry: expiry,
	}, nil
}

func configuredPresignedURLExpiry() (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(presignedURLExpiryEnv))
	if value == "" {
		return defaultPresignedURLExpiry, nil
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer number of seconds", presignedURLExpiryEnv)
	}
	expiry := time.Duration(seconds) * time.Second
	if expiry > maxPresignedURLExpiry {
		return 0, fmt.Errorf("%s must not exceed %d seconds", presignedURLExpiryEnv, int(maxPresignedURLExpiry/time.Second))
	}
	return expiry, nil
}

// ListInputFiles returns supported regular objects below the private input/
// prefix. The SDK paginator follows all ListObjectsV2 continuation tokens.
func (s *S3Client) ListInputFiles(ctx context.Context) ([]S3File, error) {
	paginator := s3.NewListObjectsV2Paginator(s.Client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.Bucket),
		Prefix: aws.String("input/"),
	})

	files := make([]S3File, 0)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list S3 input files: %w", err)
		}
		for _, object := range page.Contents {
			key := aws.ToString(object.Key)
			if isSupportedInputFile(key) {
				files = append(files, S3File{
					Name: filepath.Base(key),
					Key:  key,
				})
			}
		}
	}

	return files, nil
}

// HeadInputObject verifies that an input object exists without downloading it.
func (s *S3Client) HeadInputObject(ctx context.Context, key string) error {
	_, err := s.Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("head S3 input object %q: %w", key, err)
	}
	return nil
}

// DownloadInputObject opens an existing input object for streaming download.
func (s *S3Client) DownloadInputObject(ctx context.Context, key string) (io.ReadCloser, error) {
	output, err := s.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("get S3 input object %q: %w", key, err)
	}
	return output.Body, nil
}

// UploadOutputObject writes a private processed-job artifact to S3.
func (s *S3Client) UploadOutputObject(ctx context.Context, key string, body io.Reader, contentType string) error {
	_, err := s.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.Bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("put S3 output object %q: %w", key, err)
	}
	return nil
}

// PresignOutputGet creates a short-lived URL for one private output object.
func (s *S3Client) PresignOutputGet(ctx context.Context, key string) (string, time.Duration, error) {
	if err := validateOutputKey(key); err != nil {
		return "", 0, err
	}
	if s.Presigner == nil {
		if s.Client == nil {
			return "", 0, fmt.Errorf("S3 client is not configured")
		}
		s.Presigner = s3.NewPresignClient(s.Client)
	}
	expiry := s.PresignedExpiry
	if expiry <= 0 {
		expiry = defaultPresignedURLExpiry
	}
	presigned, err := s.Presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	}, func(options *s3.PresignOptions) {
		options.Expires = expiry
	})
	if err != nil {
		return "", 0, fmt.Errorf("presign S3 output object: %w", err)
	}
	return presigned.URL, expiry, nil
}

func isSupportedInputFile(key string) bool {
	if !strings.HasPrefix(key, "input/") || key == "input/" || strings.HasSuffix(key, "/") {
		return false
	}
	switch strings.ToLower(filepath.Ext(key)) {
	case ".csv", ".log", ".png", ".jpg", ".jpeg", ".gif":
		return true
	default:
		return false
	}
}
