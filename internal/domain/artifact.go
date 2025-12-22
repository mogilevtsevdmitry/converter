package domain

import (
	"time"

	"github.com/google/uuid"
)

// ArtifactType represents the type of artifact
type ArtifactType string

const (
	ArtifactTypeHLSMaster   ArtifactType = "HLS_MASTER"
	ArtifactTypeHLSVariant  ArtifactType = "HLS_VARIANT"
	ArtifactTypeSegment     ArtifactType = "SEGMENT"
	ArtifactTypeSubtitle    ArtifactType = "SUBTITLE"
	ArtifactTypeThumbTile   ArtifactType = "THUMB_TILE"
	ArtifactTypeThumbVTT    ArtifactType = "THUMB_VTT"
	ArtifactTypeMetadataJSON ArtifactType = "METADATA_JSON"
)

// Artifact represents an output artifact from the conversion process
type Artifact struct {
	ID        uuid.UUID    `json:"id" db:"id"`
	JobID     uuid.UUID    `json:"jobId" db:"job_id"`
	Type      ArtifactType `json:"type" db:"type"`
	Bucket    string       `json:"bucket" db:"bucket"`
	Key       string       `json:"key" db:"key"`
	SizeBytes *int64       `json:"sizeBytes,omitempty" db:"size_bytes"`
	Checksum  *string      `json:"checksum,omitempty" db:"checksum"`
	CreatedAt time.Time    `json:"createdAt" db:"created_at"`
}

// NewArtifact creates a new artifact
func NewArtifact(jobID uuid.UUID, artifactType ArtifactType, bucket, key string) *Artifact {
	return &Artifact{
		ID:        uuid.New(),
		JobID:     jobID,
		Type:      artifactType,
		Bucket:    bucket,
		Key:       key,
		CreatedAt: time.Now().UTC(),
	}
}

// WithSize sets the size of the artifact
func (a *Artifact) WithSize(size int64) *Artifact {
	a.SizeBytes = &size
	return a
}

// WithChecksum sets the checksum of the artifact
func (a *Artifact) WithChecksum(checksum string) *Artifact {
	a.Checksum = &checksum
	return a
}
