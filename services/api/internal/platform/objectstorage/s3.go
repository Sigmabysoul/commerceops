// This file implements the object-storage contract with an S3-compatible remote service in the object-storage infrastructure layer.
package objectstorage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

type S3Options struct {
	Endpoint  string
	Bucket    string
	Region    string
	AccessKey string
	SecretKey string
	PathStyle bool
}

type S3 struct {
	client *s3.Client
	bucket string
}

func NewS3(ctx context.Context, options S3Options) (*S3, error) {
	if err := validateS3Options(options); err != nil {
		return nil, err
	}

	configuration, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(options.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(options.AccessKey, options.SecretKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("load S3 configuration: %w", err)
	}
	// Some S3-compatible providers do not implement optional AWS checksum
	// extensions. SigV4 and provider-required checksums remain enabled.
	configuration.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
	configuration.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired

	client := s3.NewFromConfig(configuration, func(clientOptions *s3.Options) {
		clientOptions.UsePathStyle = options.PathStyle
		if options.Endpoint != "" {
			clientOptions.BaseEndpoint = aws.String(options.Endpoint)
		}
	})
	return &S3{client: client, bucket: options.Bucket}, nil
}

func (s *S3) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("object key is required")
	}
	if size < 0 {
		return errors.New("object size cannot be negative")
	}
	input := &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentLength: aws.Int64(size),
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}
	if _, err := s.client.PutObject(ctx, input); err != nil {
		return fmt.Errorf("put S3 object: %w", err)
	}
	return nil
}

func (s *S3) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("object key is required")
	}
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var noSuchKey *types.NoSuchKey
		var responseError *smithyhttp.ResponseError
		if errors.As(err, &noSuchKey) || (errors.As(err, &responseError) && responseError.HTTPStatusCode() == http.StatusNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get S3 object: %w", err)
	}
	return output.Body, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("object key is required")
	}
	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}); err != nil {
		return fmt.Errorf("delete S3 object: %w", err)
	}
	return nil
}

func validateS3Options(options S3Options) error {
	required := []struct {
		name  string
		value string
	}{
		{"bucket", options.Bucket},
		{"region", options.Region},
		{"access key", options.AccessKey},
		{"secret key", options.SecretKey},
	}
	for _, setting := range required {
		if strings.TrimSpace(setting.value) == "" {
			return fmt.Errorf("S3 %s is required", setting.name)
		}
	}
	if options.Endpoint != "" {
		endpoint, err := url.Parse(options.Endpoint)
		if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" {
			return errors.New("S3 endpoint must be an absolute HTTP(S) URL")
		}
	}
	return nil
}
