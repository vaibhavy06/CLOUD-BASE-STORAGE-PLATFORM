package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var MinioClient *minio.Client
const DefaultBucketName = "cloudstore"

// InitMinio initializes the MinIO S3 SDK client and ensures the default bucket exists
func InitMinio(endpoint, accessKey, secretKey string) (*minio.Client, error) {
	// Initialize minio client object.
	// We check if we need to use SSL. In docker-compose it's local HTTP, so secure=false
	useSSL := false
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create minio client: %w", err)
	}

	// Make a simple check by pinging list buckets
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	buckets, err := client.ListBuckets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list minio buckets: %w", err)
	}
	log.Printf("Connected to MinIO successfully. Found %d bucket(s).", len(buckets))

	MinioClient = client

	// Ensure the default bucket exists
	err = EnsureBucket(ctx, DefaultBucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure default bucket exists: %w", err)
	}

	return MinioClient, nil
}

// EnsureBucket checks if a bucket exists and creates it if it doesn't
func EnsureBucket(ctx context.Context, bucketName string) error {
	if MinioClient == nil {
		return fmt.Errorf("minio client is not initialized")
	}

	exists, err := MinioClient.BucketExists(ctx, bucketName)
	if err != nil {
		return err
	}

	if !exists {
		log.Printf("Bucket '%s' does not exist. Creating it...", bucketName)
		err = MinioClient.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
		if err != nil {
			return err
		}
		log.Printf("Bucket '%s' created successfully.", bucketName)
	}

	return nil
}

// PutObject uploads an object from a reader to MinIO
func PutObject(ctx context.Context, objectName string, reader io.Reader, size int64, contentType string) error {
	if MinioClient == nil {
		return fmt.Errorf("minio client is not initialized")
	}

	_, err := MinioClient.PutObject(ctx, DefaultBucketName, objectName, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("failed to put object in minio: %w", err)
	}

	return nil
}

// GetObject retrieves an object from MinIO
func GetObject(ctx context.Context, objectName string) (io.ReadCloser, error) {
	if MinioClient == nil {
		return nil, fmt.Errorf("minio client is not initialized")
	}

	object, err := MinioClient.GetObject(ctx, DefaultBucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object from minio: %w", err)
	}

	return object, nil
}

// DeleteObject deletes an object from MinIO
func DeleteObject(ctx context.Context, objectName string) error {
	if MinioClient == nil {
		return fmt.Errorf("minio client is not initialized")
	}

	err := MinioClient.RemoveObject(ctx, DefaultBucketName, objectName, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to remove object from minio: %w", err)
	}

	return nil
}

// CopyObject copies an object from one path to another inside the default bucket
func CopyObject(ctx context.Context, srcKey, destKey string) error {
	if MinioClient == nil {
		return fmt.Errorf("minio client is not initialized")
	}

	srcOpts := minio.CopySrcOptions{
		Bucket: DefaultBucketName,
		Object: srcKey,
	}

	destOpts := minio.CopyDestOptions{
		Bucket: DefaultBucketName,
		Object: destKey,
	}

	_, err := MinioClient.CopyObject(ctx, destOpts, srcOpts)
	if err != nil {
		return fmt.Errorf("failed to copy object inside minio: %w", err)
	}

	return nil
}

// GetPresignedDownloadURL generates a secure presigned URL to download an object
func GetPresignedDownloadURL(ctx context.Context, objectName string, filename string, expiry time.Duration) (string, error) {
	if MinioClient == nil {
		return "", fmt.Errorf("minio client is not initialized")
	}

	reqParams := make(url.Values)
	reqParams.Set("response-content-disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	presignedURL, err := MinioClient.PresignedGetObject(ctx, DefaultBucketName, objectName, expiry, reqParams)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return presignedURL.String(), nil
}
