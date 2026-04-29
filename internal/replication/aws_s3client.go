package replication

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// AWSS3Client is the production [S3Client] implementation built on top
// of aws-sdk-go-v2. It works against AWS S3, MinIO, RustFS, Wasabi and
// any other S3-compatible store; the endpoint URL and credentials are
// supplied at construction time (typically by the per-run secrets
// loader in cmd/nova-api).
//
// One AWSS3Client wraps one configured *s3.Client. Path-style addressing
// is forced because most on-prem S3 implementations (RustFS/MinIO) do
// not support the virtual-hosted form.
type AWSS3Client struct {
	cl *s3.Client
}

// AWSS3Config is the wiring for [NewAWSS3Client].
type AWSS3Config struct {
	// Region is the S3 region (mandatory for AWS, often arbitrary for
	// on-prem). Defaults to "us-east-1" when empty.
	Region string
	// Endpoint, when non-empty, overrides the SDK-default endpoint
	// resolution; required for MinIO/RustFS deployments.
	Endpoint string
	// AccessKey + SecretKey are the static credentials. Both must be set.
	AccessKey string
	SecretKey string
}

// NewAWSS3Client builds an aws-sdk-go-v2 backed S3 client from
// already-resolved credentials and endpoint config.
func NewAWSS3Client(ctx context.Context, c AWSS3Config) (*AWSS3Client, error) {
	region := c.Region
	if region == "" {
		region = "us-east-1"
	}
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(c.AccessKey, c.SecretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("aws s3: load config: %w", err)
	}
	cl := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if c.Endpoint != "" {
			o.BaseEndpoint = aws.String(c.Endpoint)
		}
		o.UsePathStyle = true
	})
	return &AWSS3Client{cl: cl}, nil
}

// PutObject implements [S3Client]. size may be -1 if unknown; the SDK
// will fall back to chunked uploads.
func (c *AWSS3Client) PutObject(ctx context.Context, bucket, key string, body io.Reader, size int64) error {
	in := &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   body,
	}
	if size >= 0 {
		in.ContentLength = aws.Int64(size)
	}
	_, err := c.cl.PutObject(ctx, in)
	return err
}

// GetObject implements [S3Client]. The caller must close the returned
// reader.
func (c *AWSS3Client) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	out, err := c.cl.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

// ListObjects implements [S3Client]. Paginates through all results.
func (c *AWSS3Client) ListObjects(ctx context.Context, bucket, prefix string) ([]S3Object, error) {
	var out []S3Object
	var token *string
	for {
		page, err := c.cl.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, err
		}
		for _, o := range page.Contents {
			obj := S3Object{Key: aws.ToString(o.Key)}
			if o.Size != nil {
				obj.Size = *o.Size
			}
			out = append(out, obj)
		}
		if page.IsTruncated == nil || !*page.IsTruncated {
			break
		}
		token = page.NextContinuationToken
	}
	return out, nil
}

// DeleteObject implements [S3Client].
func (c *AWSS3Client) DeleteObject(ctx context.Context, bucket, key string) error {
	_, err := c.cl.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	return err
}
