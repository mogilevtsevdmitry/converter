package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/tvoe/converter/internal/domain"
)

// ArtifactRepository handles artifact persistence
type ArtifactRepository struct {
	db *DB
}

// NewArtifactRepository creates a new artifact repository
func NewArtifactRepository(db *DB) *ArtifactRepository {
	return &ArtifactRepository{db: db}
}

// Create creates a new artifact
func (r *ArtifactRepository) Create(ctx context.Context, artifact *domain.Artifact) error {
	query := `
		INSERT INTO conversion_artifacts (
			id, job_id, type, bucket, key, size_bytes, checksum, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := r.db.Pool.Exec(ctx, query,
		artifact.ID,
		artifact.JobID,
		artifact.Type,
		artifact.Bucket,
		artifact.Key,
		artifact.SizeBytes,
		artifact.Checksum,
		artifact.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create artifact: %w", err)
	}

	return nil
}

// CreateBatch creates multiple artifacts in a transaction
func (r *ArtifactRepository) CreateBatch(ctx context.Context, artifacts []*domain.Artifact) error {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO conversion_artifacts (
			id, job_id, type, bucket, key, size_bytes, checksum, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	for _, artifact := range artifacts {
		_, err := tx.Exec(ctx, query,
			artifact.ID,
			artifact.JobID,
			artifact.Type,
			artifact.Bucket,
			artifact.Key,
			artifact.SizeBytes,
			artifact.Checksum,
			artifact.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to create artifact: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetByJobID retrieves artifacts for a job
func (r *ArtifactRepository) GetByJobID(ctx context.Context, jobID uuid.UUID) ([]*domain.Artifact, error) {
	query := `
		SELECT id, job_id, type, bucket, key, size_bytes, checksum, created_at
		FROM conversion_artifacts
		WHERE job_id = $1
		ORDER BY type, created_at
	`

	rows, err := r.db.Pool.Query(ctx, query, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get artifacts: %w", err)
	}
	defer rows.Close()

	var artifacts []*domain.Artifact
	for rows.Next() {
		var artifact domain.Artifact
		if err := rows.Scan(
			&artifact.ID,
			&artifact.JobID,
			&artifact.Type,
			&artifact.Bucket,
			&artifact.Key,
			&artifact.SizeBytes,
			&artifact.Checksum,
			&artifact.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan artifact: %w", err)
		}
		artifacts = append(artifacts, &artifact)
	}

	return artifacts, nil
}

// GetByJobIDAndType retrieves artifacts by job ID and type
func (r *ArtifactRepository) GetByJobIDAndType(ctx context.Context, jobID uuid.UUID, artifactType domain.ArtifactType) ([]*domain.Artifact, error) {
	query := `
		SELECT id, job_id, type, bucket, key, size_bytes, checksum, created_at
		FROM conversion_artifacts
		WHERE job_id = $1 AND type = $2
		ORDER BY created_at
	`

	rows, err := r.db.Pool.Query(ctx, query, jobID, artifactType)
	if err != nil {
		return nil, fmt.Errorf("failed to get artifacts: %w", err)
	}
	defer rows.Close()

	var artifacts []*domain.Artifact
	for rows.Next() {
		var artifact domain.Artifact
		if err := rows.Scan(
			&artifact.ID,
			&artifact.JobID,
			&artifact.Type,
			&artifact.Bucket,
			&artifact.Key,
			&artifact.SizeBytes,
			&artifact.Checksum,
			&artifact.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan artifact: %w", err)
		}
		artifacts = append(artifacts, &artifact)
	}

	return artifacts, nil
}

// DeleteByJobID deletes all artifacts for a job
func (r *ArtifactRepository) DeleteByJobID(ctx context.Context, jobID uuid.UUID) error {
	query := `DELETE FROM conversion_artifacts WHERE job_id = $1`

	_, err := r.db.Pool.Exec(ctx, query, jobID)
	if err != nil {
		return fmt.Errorf("failed to delete artifacts: %w", err)
	}

	return nil
}

// CountByType counts artifacts by type
func (r *ArtifactRepository) CountByType(ctx context.Context) (map[domain.ArtifactType]int, error) {
	query := `SELECT type, COUNT(*) FROM conversion_artifacts GROUP BY type`

	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to count artifacts: %w", err)
	}
	defer rows.Close()

	counts := make(map[domain.ArtifactType]int)
	for rows.Next() {
		var artifactType domain.ArtifactType
		var count int
		if err := rows.Scan(&artifactType, &count); err != nil {
			return nil, fmt.Errorf("failed to scan count: %w", err)
		}
		counts[artifactType] = count
	}

	return counts, nil
}
