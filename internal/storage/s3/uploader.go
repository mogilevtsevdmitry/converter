package s3

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/tvoe/converter/internal/domain"
)

// UploadProgress tracks upload progress
type UploadProgress struct {
	TotalFiles     int
	CompletedFiles int
	TotalBytes     int64
	UploadedBytes  int64
}

// DirectoryUploader handles uploading directories to S3
type DirectoryUploader struct {
	client         *Client
	maxConcurrent  int
	progressChan   chan UploadProgress
}

// NewDirectoryUploader creates a new directory uploader
func NewDirectoryUploader(client *Client, maxConcurrent int) *DirectoryUploader {
	return &DirectoryUploader{
		client:        client,
		maxConcurrent: maxConcurrent,
	}
}

// UploadDirectory uploads a directory to S3
func (u *DirectoryUploader) UploadDirectory(
	ctx context.Context,
	jobID uuid.UUID,
	localDir string,
	bucket string,
	prefix string,
	progressFn func(UploadProgress),
) ([]*domain.Artifact, error) {
	// Collect files to upload
	var files []fileInfo
	err := filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(localDir, path)
		if err != nil {
			return err
		}
		files = append(files, fileInfo{
			localPath: path,
			key:       filepath.Join(prefix, relPath),
			size:      info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	// Calculate total size
	var totalBytes int64
	for _, f := range files {
		totalBytes += f.size
	}

	progress := &UploadProgress{
		TotalFiles: len(files),
		TotalBytes: totalBytes,
	}

	// Upload files concurrently
	var artifacts []*domain.Artifact
	var artifactsMu sync.Mutex
	var uploadedBytes int64
	var completedFiles int32

	sem := make(chan struct{}, u.maxConcurrent)
	errChan := make(chan error, len(files))
	var wg sync.WaitGroup

	for _, f := range files {
		wg.Add(1)
		go func(f fileInfo) {
			defer wg.Done()

			select {
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			case sem <- struct{}{}:
				defer func() { <-sem }()
			}

			result, err := u.client.Upload(ctx, bucket, f.key, f.localPath)
			if err != nil {
				errChan <- fmt.Errorf("failed to upload %s: %w", f.key, err)
				return
			}

			artifact := domain.NewArtifact(jobID, determineArtifactType(f.key), bucket, f.key)
			artifact.WithSize(result.Size)
			artifact.WithChecksum(result.ETag)

			artifactsMu.Lock()
			artifacts = append(artifacts, artifact)
			artifactsMu.Unlock()

			atomic.AddInt64(&uploadedBytes, f.size)
			atomic.AddInt32(&completedFiles, 1)

			if progressFn != nil {
				progressFn(UploadProgress{
					TotalFiles:     len(files),
					CompletedFiles: int(atomic.LoadInt32(&completedFiles)),
					TotalBytes:     totalBytes,
					UploadedBytes:  atomic.LoadInt64(&uploadedBytes),
				})
			}
		}(f)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	var errs []error
	for err := range errChan {
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("upload errors: %v", errs)
	}

	_ = progress // Used for progress tracking
	return artifacts, nil
}

type fileInfo struct {
	localPath string
	key       string
	size      int64
}

// determineArtifactType determines artifact type from key
func determineArtifactType(key string) domain.ArtifactType {
	ext := filepath.Ext(key)
	base := filepath.Base(key)

	switch {
	case base == "master.m3u8":
		return domain.ArtifactTypeHLSMaster
	case ext == ".m3u8":
		return domain.ArtifactTypeHLSVariant
	case ext == ".ts":
		return domain.ArtifactTypeSegment
	case ext == ".vtt" && filepath.Dir(key) == "thumbs":
		return domain.ArtifactTypeThumbVTT
	case ext == ".vtt":
		return domain.ArtifactTypeSubtitle
	case ext == ".jpg" || ext == ".png":
		return domain.ArtifactTypeThumbTile
	case ext == ".json":
		return domain.ArtifactTypeMetadataJSON
	default:
		return domain.ArtifactTypeSegment
	}
}
