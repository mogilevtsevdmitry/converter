package s3

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/tvoe/converter/internal/config"
)

const (
	// MinPartSize is the minimum part size for multipart upload (5MB)
	MinPartSize = 5 * 1024 * 1024
	// MaxPartSize is the maximum part size for multipart upload (100MB)
	MaxPartSize = 100 * 1024 * 1024
	// DefaultPartSize is the default part size (50MB)
	DefaultPartSize = 50 * 1024 * 1024
)

// Client wraps S3 operations
type Client struct {
	client     *s3.Client
	bucket     string
	maxRetries int
}

// New creates a new S3 client
func New(cfg config.S3Config) (*Client, error) {
	customResolver := aws.EndpointResolverWithOptionsFunc(
		func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:               cfg.Endpoint,
				HostnameImmutable: true,
				SigningRegion:     cfg.Region,
			}, nil
		},
	)

	awsCfg := aws.Config{
		Region:                      cfg.Region,
		Credentials:                 credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		EndpointResolverWithOptions: customResolver,
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	return &Client{
		client:     client,
		bucket:     cfg.BucketOutput,
		maxRetries: 3,
	}, nil
}

// Download downloads a file from S3
func (c *Client) Download(ctx context.Context, bucket, key, destPath string) error {
	output, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to get object: %w", err)
	}
	defer output.Body.Close()

	// Create destination directory
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, output.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// Upload uploads a file to S3 using multipart upload for large files
func (c *Client) Upload(ctx context.Context, bucket, key, srcPath string) (*UploadResult, error) {
	file, err := os.Open(srcPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	size := stat.Size()
	if size < MinPartSize {
		return c.uploadSimple(ctx, bucket, key, file, size)
	}

	return c.uploadMultipart(ctx, bucket, key, file, size)
}

// uploadSimple uploads a small file in a single request
func (c *Client) uploadSimple(ctx context.Context, bucket, key string, file *os.File, size int64) (*UploadResult, error) {
	contentType := detectContentType(key)

	output, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          file,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to upload: %w", err)
	}

	return &UploadResult{
		Bucket:   bucket,
		Key:      key,
		ETag:     aws.ToString(output.ETag),
		Size:     size,
	}, nil
}

// uploadMultipart uploads a large file using multipart upload
func (c *Client) uploadMultipart(ctx context.Context, bucket, key string, file *os.File, size int64) (*UploadResult, error) {
	contentType := detectContentType(key)

	// Initiate multipart upload
	createOutput, err := c.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create multipart upload: %w", err)
	}

	uploadID := aws.ToString(createOutput.UploadId)

	// Calculate part size and count
	partSize := int64(DefaultPartSize)
	partCount := (size + partSize - 1) / partSize
	if partCount > 10000 {
		partSize = (size + 9999) / 10000
	}

	var completedParts []types.CompletedPart
	var partsMu sync.Mutex

	// Upload parts
	for partNum := int64(1); partNum <= partCount; partNum++ {
		offset := (partNum - 1) * partSize
		remaining := size - offset
		currentPartSize := partSize
		if remaining < partSize {
			currentPartSize = remaining
		}

		partData := make([]byte, currentPartSize)
		n, err := file.ReadAt(partData, offset)
		if err != nil && err != io.EOF {
			c.abortMultipartUpload(ctx, bucket, key, uploadID)
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		partData = partData[:n]

		var uploadErr error
		for retry := 0; retry < c.maxRetries; retry++ {
			partOutput, err := c.client.UploadPart(ctx, &s3.UploadPartInput{
				Bucket:     aws.String(bucket),
				Key:        aws.String(key),
				UploadId:   aws.String(uploadID),
				PartNumber: aws.Int32(int32(partNum)),
				Body:       NewSectionReader(partData),
			})
			if err != nil {
				uploadErr = err
				time.Sleep(time.Duration(retry+1) * time.Second)
				continue
			}

			partsMu.Lock()
			completedParts = append(completedParts, types.CompletedPart{
				ETag:       partOutput.ETag,
				PartNumber: aws.Int32(int32(partNum)),
			})
			partsMu.Unlock()
			uploadErr = nil
			break
		}

		if uploadErr != nil {
			c.abortMultipartUpload(ctx, bucket, key, uploadID)
			return nil, fmt.Errorf("failed to upload part %d: %w", partNum, uploadErr)
		}
	}

	// Complete multipart upload
	completeOutput, err := c.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		c.abortMultipartUpload(ctx, bucket, key, uploadID)
		return nil, fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	return &UploadResult{
		Bucket: bucket,
		Key:    key,
		ETag:   aws.ToString(completeOutput.ETag),
		Size:   size,
	}, nil
}

// abortMultipartUpload aborts a multipart upload
func (c *Client) abortMultipartUpload(ctx context.Context, bucket, key, uploadID string) {
	c.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	})
}

// Delete deletes an object from S3
func (c *Client) Delete(ctx context.Context, bucket, key string) error {
	_, err := c.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}
	return nil
}

// Exists checks if an object exists in S3
func (c *Client) Exists(ctx context.Context, bucket, key string) (bool, error) {
	_, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return false, nil
	}
	return true, nil
}

// ListObjects lists objects with a given prefix
func (c *Client) ListObjects(ctx context.Context, bucket, prefix string) ([]ObjectInfo, error) {
	var objects []ObjectInfo

	paginator := s3.NewListObjectsV2Paginator(c.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}

		for _, obj := range page.Contents {
			objects = append(objects, ObjectInfo{
				Key:          aws.ToString(obj.Key),
				Size:         aws.ToInt64(obj.Size),
				LastModified: aws.ToTime(obj.LastModified),
				ETag:         aws.ToString(obj.ETag),
			})
		}
	}

	return objects, nil
}

// Health checks S3 connectivity
func (c *Client) Health(ctx context.Context) error {
	_, err := c.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(c.bucket),
	})
	return err
}

// GetDefaultBucket returns the default output bucket
func (c *Client) GetDefaultBucket() string {
	return c.bucket
}

// UploadResult holds the result of an upload operation
type UploadResult struct {
	Bucket string
	Key    string
	ETag   string
	Size   int64
}

// ObjectInfo holds information about an S3 object
type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
	ETag         string
}

// SectionReader wraps a byte slice for reading
type SectionReader struct {
	data   []byte
	offset int
}

// NewSectionReader creates a new section reader
func NewSectionReader(data []byte) *SectionReader {
	return &SectionReader{data: data}
}

// Read implements io.Reader
func (r *SectionReader) Read(p []byte) (int, error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

// detectContentType returns content type based on file extension
func detectContentType(key string) string {
	ext := filepath.Ext(key)
	contentTypes := map[string]string{
		".m3u8": "application/vnd.apple.mpegurl",
		".ts":   "video/mp2t",
		".mp4":  "video/mp4",
		".vtt":  "text/vtt",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".json": "application/json",
	}
	if ct, ok := contentTypes[ext]; ok {
		return ct
	}
	return "application/octet-stream"
}
