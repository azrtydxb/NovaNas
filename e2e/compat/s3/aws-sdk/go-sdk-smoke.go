// go-sdk-smoke.go — AWS SDK for Go v2 smoke test against NovaNas s3gw.
//
// Build/run:
//   cd e2e/compat/s3/aws-sdk
//   go mod init novanas-s3-gosdk && go mod tidy
//   go run go-sdk-smoke.go
//
// Exercises PUT/GET/LIST/MULTIPART/DELETE. Exits non-zero on failure.

//go:build ignore
// +build ignore

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	endpoint := env("S3_ENDPOINT", "https://localhost:9000")
	access := env("S3_ACCESS_KEY", "novanas")
	secret := env("S3_SECRET_KEY", "novanas-secret")
	region := env("S3_REGION", "us-east-1")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	httpClient := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}}

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(access, secret, "")),
		config.WithHTTPClient(httpClient),
	)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	cli := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
		o.BaseEndpoint = aws.String(endpoint)
	})

	bucket := fmt.Sprintf("e2e-gosdk-%d", time.Now().UnixNano())
	if _, err := cli.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &bucket}); err != nil {
		log.Fatalf("CreateBucket: %v", err)
	}
	defer cleanup(ctx, cli, bucket)

	key := "hello.txt"
	payload := []byte(fmt.Sprintf("hello from go-sdk %d", time.Now().UnixNano()))
	if _, err := cli.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &bucket, Key: &key, Body: bytes.NewReader(payload),
	}); err != nil {
		log.Fatalf("PutObject: %v", err)
	}

	out, err := cli.GetObject(ctx, &s3.GetObjectInput{Bucket: &bucket, Key: &key})
	if err != nil {
		log.Fatalf("GetObject: %v", err)
	}
	got, _ := io.ReadAll(out.Body)
	out.Body.Close()
	if !bytes.Equal(got, payload) {
		log.Fatalf("GET payload mismatch")
	}

	list, err := cli.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: &bucket})
	if err != nil {
		log.Fatalf("ListObjectsV2: %v", err)
	}
	found := false
	for _, o := range list.Contents {
		if aws.ToString(o.Key) == key {
			found = true
		}
	}
	if !found {
		log.Fatalf("LIST missing key")
	}

	// Multipart (10 MiB) via the manager uploader.
	mkey := "multipart.bin"
	big := make([]byte, 10*1024*1024)
	if _, err := rand.Read(big); err != nil {
		log.Fatalf("rand: %v", err)
	}
	uploader := manager.NewUploader(cli, func(u *manager.Uploader) {
		u.PartSize = 5 * 1024 * 1024
	})
	if _, err := uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: &bucket, Key: &mkey, Body: bytes.NewReader(big),
	}); err != nil {
		log.Fatalf("multipart upload: %v", err)
	}
	head, err := cli.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &bucket, Key: &mkey})
	if err != nil {
		log.Fatalf("HeadObject: %v", err)
	}
	if aws.ToInt64(head.ContentLength) != int64(len(big)) {
		log.Fatalf("multipart size mismatch")
	}
	fmt.Println("go-sdk-smoke: PASS")
}

func cleanup(ctx context.Context, cli *s3.Client, bucket string) {
	list, err := cli.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: &bucket})
	if err != nil {
		return
	}
	for _, o := range list.Contents {
		_, _ = cli.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &bucket, Key: o.Key})
	}
	_, _ = cli.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: &bucket})
}
