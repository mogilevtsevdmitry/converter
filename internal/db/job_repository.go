package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/tvoe/converter/internal/domain"
)

// ErrNotFound is returned when a resource is not found
var ErrNotFound = errors.New("not found")

// ErrConcurrentModification is returned on optimistic lock failure
var ErrConcurrentModification = errors.New("concurrent modification")

// JobRepository handles job persistence
type JobRepository struct {
	db *DB
}

// NewJobRepository creates a new job repository
func NewJobRepository(db *DB) *JobRepository {
	return &JobRepository{db: db}
}

// Create creates a new job
func (r *JobRepository) Create(ctx context.Context, job *domain.Job) error {
	profileJSON, err := json.Marshal(job.Profile)
	if err != nil {
		return fmt.Errorf("failed to marshal profile: %w", err)
	}

	query := `
		INSERT INTO conversion_jobs (
			id, video_id, source_bucket, source_key, status, current_stage,
			stage_progress, overall_progress, profile, idempotency_key,
			workflow_id, priority, created_at, started_at, updated_at,
			finished_at, attempt, last_error_id, lock_version
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19
		)
	`

	_, err = r.db.Pool.Exec(ctx, query,
		job.ID,
		job.VideoID,
		job.SourceBucket,
		job.SourceKey,
		job.Status,
		job.CurrentStage,
		job.StageProgress,
		job.OverallProgress,
		profileJSON,
		job.IdempotencyKey,
		job.WorkflowID,
		job.Priority,
		job.CreatedAt,
		job.StartedAt,
		job.UpdatedAt,
		job.FinishedAt,
		job.Attempt,
		job.LastErrorID,
		job.LockVersion,
	)
	if err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}

	return nil
}

// GetByID retrieves a job by ID
func (r *JobRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Job, error) {
	query := `
		SELECT id, video_id, source_bucket, source_key, status, current_stage,
			stage_progress, overall_progress, profile, idempotency_key,
			workflow_id, priority, created_at, started_at, updated_at,
			finished_at, attempt, last_error_id, lock_version
		FROM conversion_jobs
		WHERE id = $1
	`

	return r.scanJob(r.db.Pool.QueryRow(ctx, query, id))
}

// GetByIdempotencyKey retrieves a job by idempotency key
func (r *JobRepository) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Job, error) {
	query := `
		SELECT id, video_id, source_bucket, source_key, status, current_stage,
			stage_progress, overall_progress, profile, idempotency_key,
			workflow_id, priority, created_at, started_at, updated_at,
			finished_at, attempt, last_error_id, lock_version
		FROM conversion_jobs
		WHERE idempotency_key = $1
	`

	return r.scanJob(r.db.Pool.QueryRow(ctx, query, key))
}

// Update updates a job with optimistic locking
func (r *JobRepository) Update(ctx context.Context, job *domain.Job) error {
	profileJSON, err := json.Marshal(job.Profile)
	if err != nil {
		return fmt.Errorf("failed to marshal profile: %w", err)
	}

	query := `
		UPDATE conversion_jobs SET
			video_id = $2,
			source_bucket = $3,
			source_key = $4,
			status = $5,
			current_stage = $6,
			stage_progress = $7,
			overall_progress = $8,
			profile = $9,
			idempotency_key = $10,
			workflow_id = $11,
			priority = $12,
			started_at = $13,
			finished_at = $14,
			attempt = $15,
			last_error_id = $16,
			lock_version = lock_version + 1
		WHERE id = $1 AND lock_version = $17
	`

	result, err := r.db.Pool.Exec(ctx, query,
		job.ID,
		job.VideoID,
		job.SourceBucket,
		job.SourceKey,
		job.Status,
		job.CurrentStage,
		job.StageProgress,
		job.OverallProgress,
		profileJSON,
		job.IdempotencyKey,
		job.WorkflowID,
		job.Priority,
		job.StartedAt,
		job.FinishedAt,
		job.Attempt,
		job.LastErrorID,
		job.LockVersion,
	)
	if err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrConcurrentModification
	}

	job.LockVersion++
	return nil
}

// UpdateProgress updates job progress
func (r *JobRepository) UpdateProgress(ctx context.Context, jobID uuid.UUID, stage domain.Stage, stageProgress, overallProgress int) error {
	query := `
		UPDATE conversion_jobs SET
			current_stage = $2,
			stage_progress = $3,
			overall_progress = $4
		WHERE id = $1
	`

	_, err := r.db.Pool.Exec(ctx, query, jobID, stage, stageProgress, overallProgress)
	if err != nil {
		return fmt.Errorf("failed to update progress: %w", err)
	}

	return nil
}

// UpdateStatus updates job status
func (r *JobRepository) UpdateStatus(ctx context.Context, jobID uuid.UUID, status domain.JobStatus) error {
	query := `UPDATE conversion_jobs SET status = $2 WHERE id = $1`

	_, err := r.db.Pool.Exec(ctx, query, jobID, status)
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	return nil
}

// SetStarted marks job as started
func (r *JobRepository) SetStarted(ctx context.Context, jobID uuid.UUID) error {
	query := `
		UPDATE conversion_jobs SET
			status = $2,
			started_at = $3
		WHERE id = $1
	`

	_, err := r.db.Pool.Exec(ctx, query, jobID, domain.JobStatusRunning, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to set started: %w", err)
	}

	return nil
}

// SetFinished marks job as finished
func (r *JobRepository) SetFinished(ctx context.Context, jobID uuid.UUID, status domain.JobStatus) error {
	query := `
		UPDATE conversion_jobs SET
			status = $2,
			finished_at = $3,
			overall_progress = CASE WHEN $2 = 'COMPLETED' THEN 100 ELSE overall_progress END
		WHERE id = $1
	`

	_, err := r.db.Pool.Exec(ctx, query, jobID, status, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to set finished: %w", err)
	}

	return nil
}

// SetWorkflowID sets the Temporal workflow ID
func (r *JobRepository) SetWorkflowID(ctx context.Context, jobID uuid.UUID, workflowID string) error {
	query := `UPDATE conversion_jobs SET workflow_id = $2 WHERE id = $1`

	_, err := r.db.Pool.Exec(ctx, query, jobID, workflowID)
	if err != nil {
		return fmt.Errorf("failed to set workflow ID: %w", err)
	}

	return nil
}

// ListByStatus lists jobs by status
func (r *JobRepository) ListByStatus(ctx context.Context, status domain.JobStatus, limit int) ([]*domain.Job, error) {
	query := `
		SELECT id, video_id, source_bucket, source_key, status, current_stage,
			stage_progress, overall_progress, profile, idempotency_key,
			workflow_id, priority, created_at, started_at, updated_at,
			finished_at, attempt, last_error_id, lock_version
		FROM conversion_jobs
		WHERE status = $1
		ORDER BY priority DESC, created_at ASC
		LIMIT $2
	`

	rows, err := r.db.Pool.Query(ctx, query, status, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*domain.Job
	for rows.Next() {
		job, err := r.scanJobFromRows(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// CountByStatus counts jobs by status
func (r *JobRepository) CountByStatus(ctx context.Context) (map[domain.JobStatus]int, error) {
	query := `SELECT status, COUNT(*) FROM conversion_jobs GROUP BY status`

	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to count jobs: %w", err)
	}
	defer rows.Close()

	counts := make(map[domain.JobStatus]int)
	for rows.Next() {
		var status domain.JobStatus
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan count: %w", err)
		}
		counts[status] = count
	}

	return counts, nil
}

func (r *JobRepository) scanJob(row pgx.Row) (*domain.Job, error) {
	var job domain.Job
	var profileJSON []byte

	err := row.Scan(
		&job.ID,
		&job.VideoID,
		&job.SourceBucket,
		&job.SourceKey,
		&job.Status,
		&job.CurrentStage,
		&job.StageProgress,
		&job.OverallProgress,
		&profileJSON,
		&job.IdempotencyKey,
		&job.WorkflowID,
		&job.Priority,
		&job.CreatedAt,
		&job.StartedAt,
		&job.UpdatedAt,
		&job.FinishedAt,
		&job.Attempt,
		&job.LastErrorID,
		&job.LockVersion,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to scan job: %w", err)
	}

	if err := json.Unmarshal(profileJSON, &job.Profile); err != nil {
		return nil, fmt.Errorf("failed to unmarshal profile: %w", err)
	}

	return &job, nil
}

func (r *JobRepository) scanJobFromRows(rows pgx.Rows) (*domain.Job, error) {
	var job domain.Job
	var profileJSON []byte

	err := rows.Scan(
		&job.ID,
		&job.VideoID,
		&job.SourceBucket,
		&job.SourceKey,
		&job.Status,
		&job.CurrentStage,
		&job.StageProgress,
		&job.OverallProgress,
		&profileJSON,
		&job.IdempotencyKey,
		&job.WorkflowID,
		&job.Priority,
		&job.CreatedAt,
		&job.StartedAt,
		&job.UpdatedAt,
		&job.FinishedAt,
		&job.Attempt,
		&job.LastErrorID,
		&job.LockVersion,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan job: %w", err)
	}

	if err := json.Unmarshal(profileJSON, &job.Profile); err != nil {
		return nil, fmt.Errorf("failed to unmarshal profile: %w", err)
	}

	return &job, nil
}
