package ffmpeg

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// Workspace manages local file storage for a job
type Workspace struct {
	root   string
	jobID  uuid.UUID
	paths  WorkspacePaths
}

// WorkspacePaths holds all workspace directory paths
type WorkspacePaths struct {
	Root       string
	Input      string
	Meta       string
	Transcoded string
	Subtitles  string
	Thumbs     string
	HLS        string
}

// NewWorkspace creates a new workspace for a job
func NewWorkspace(root string, jobID uuid.UUID) *Workspace {
	jobDir := filepath.Join(root, jobID.String())
	return &Workspace{
		root:  root,
		jobID: jobID,
		paths: WorkspacePaths{
			Root:       jobDir,
			Input:      filepath.Join(jobDir, "input"),
			Meta:       filepath.Join(jobDir, "meta"),
			Transcoded: filepath.Join(jobDir, "transcoded"),
			Subtitles:  filepath.Join(jobDir, "subtitles"),
			Thumbs:     filepath.Join(jobDir, "thumbs"),
			HLS:        filepath.Join(jobDir, "hls"),
		},
	}
}

// Create creates all workspace directories
func (w *Workspace) Create() error {
	dirs := []string{
		w.paths.Input,
		w.paths.Meta,
		w.paths.Transcoded,
		w.paths.Subtitles,
		w.paths.Thumbs,
		w.paths.HLS,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Create lock file
	lockPath := filepath.Join(w.paths.Root, ".lock")
	lockFile, err := os.Create(lockPath)
	if err != nil {
		return fmt.Errorf("failed to create lock file: %w", err)
	}
	lockFile.Close()

	return nil
}

// Cleanup removes the workspace
func (w *Workspace) Cleanup() error {
	return os.RemoveAll(w.paths.Root)
}

// Paths returns workspace paths
func (w *Workspace) Paths() WorkspacePaths {
	return w.paths
}

// InputPath returns path for input file
func (w *Workspace) InputPath(filename string) string {
	return filepath.Join(w.paths.Input, filename)
}

// TranscodedPath returns path for transcoded file
func (w *Workspace) TranscodedPath(quality string) string {
	return filepath.Join(w.paths.Transcoded, quality+".mp4")
}

// SubtitlePath returns path for subtitle file
func (w *Workspace) SubtitlePath(lang string) string {
	return filepath.Join(w.paths.Subtitles, lang+".vtt")
}

// ThumbnailPath returns path for thumbnail
func (w *Workspace) ThumbnailPath(index int) string {
	return filepath.Join(w.paths.Thumbs, fmt.Sprintf("thumb_%05d.jpg", index))
}

// TilePath returns path for thumbnail tile
func (w *Workspace) TilePath(index int) string {
	return filepath.Join(w.paths.Thumbs, fmt.Sprintf("tile_%03d.jpg", index))
}

// HLSPath returns path for HLS directory
func (w *Workspace) HLSPath() string {
	return w.paths.HLS
}

// MetaPath returns path for metadata file
func (w *Workspace) MetaPath(filename string) string {
	return filepath.Join(w.paths.Meta, filename)
}

// Exists checks if workspace exists
func (w *Workspace) Exists() bool {
	_, err := os.Stat(w.paths.Root)
	return err == nil
}

// IsLocked checks if workspace is locked
func (w *Workspace) IsLocked() bool {
	lockPath := filepath.Join(w.paths.Root, ".lock")
	_, err := os.Stat(lockPath)
	return err == nil
}

// GetDiskUsage returns workspace disk usage in bytes
func (w *Workspace) GetDiskUsage() (int64, error) {
	var size int64
	err := filepath.Walk(w.paths.Root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// CleanupOrphans removes workspaces older than maxAge that are not locked
func CleanupOrphans(root string, maxAge time.Duration) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("failed to read workspace root: %w", err)
	}

	cutoff := time.Now().Add(-maxAge)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Parse job ID to validate it's a workspace
		if _, err := uuid.Parse(entry.Name()); err != nil {
			continue
		}

		dirPath := filepath.Join(root, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Skip if too recent
		if info.ModTime().After(cutoff) {
			continue
		}

		// Skip if locked
		lockPath := filepath.Join(dirPath, ".lock")
		if _, err := os.Stat(lockPath); err == nil {
			continue
		}

		// Remove orphan workspace
		os.RemoveAll(dirPath)
	}

	return nil
}
