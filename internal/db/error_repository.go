package db

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/tvoe/converter/internal/domain"
)

// ErrorRepository handles conversion error persistence
type ErrorRepository struct {
	db *DB
}

// NewErrorRepository creates a new error repository
func NewErrorRepository(db *DB) *ErrorRepository {
	return &ErrorRepository{db: db}
}

// Create creates a new conversion error
func (r *ErrorRepository) Create(ctx context.Context, convErr *domain.ConversionError) error {
	detailsJSON, err := json.Marshal(convErr.Details)
	if err != nil {
		return fmt.Errorf("failed to marshal details: %w", err)
	}

	query := `
		INSERT INTO conversion_errors (
			id, job_id, stage, class, code, message, details, attempt, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err = r.db.Pool.Exec(ctx, query,
		convErr.ID,
		convErr.JobID,
		convErr.Stage,
		convErr.Class,
		convErr.Code,
		convErr.Message,
		detailsJSON,
		convErr.Attempt,
		convErr.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create error: %w", err)
	}

	// Update last_error_id on job
	updateQuery := `UPDATE conversion_jobs SET last_error_id = $2 WHERE id = $1`
	_, err = r.db.Pool.Exec(ctx, updateQuery, convErr.JobID, convErr.ID)
	if err != nil {
		return fmt.Errorf("failed to update job last_error_id: %w", err)
	}

	return nil
}

// GetByJobID retrieves errors for a job
func (r *ErrorRepository) GetByJobID(ctx context.Context, jobID uuid.UUID) ([]*domain.ConversionError, error) {
	query := `
		SELECT id, job_id, stage, class, code, message, details, attempt, created_at
		FROM conversion_errors
		WHERE job_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Pool.Query(ctx, query, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get errors: %w", err)
	}
	defer rows.Close()

	var errors []*domain.ConversionError
	for rows.Next() {
		var convErr domain.ConversionError
		var detailsJSON []byte

		if err := rows.Scan(
			&convErr.ID,
			&convErr.JobID,
			&convErr.Stage,
			&convErr.Class,
			&convErr.Code,
			&convErr.Message,
			&detailsJSON,
			&convErr.Attempt,
			&convErr.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan error: %w", err)
		}

		if err := json.Unmarshal(detailsJSON, &convErr.Details); err != nil {
			convErr.Details = make(map[string]any)
		}

		errors = append(errors, &convErr)
	}

	return errors, nil
}

// GetLatestByJobID retrieves the latest error for a job
func (r *ErrorRepository) GetLatestByJobID(ctx context.Context, jobID uuid.UUID) (*domain.ConversionError, error) {
	query := `
		SELECT id, job_id, stage, class, code, message, details, attempt, created_at
		FROM conversion_errors
		WHERE job_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`

	var convErr domain.ConversionError
	var detailsJSON []byte

	err := r.db.Pool.QueryRow(ctx, query, jobID).Scan(
		&convErr.ID,
		&convErr.JobID,
		&convErr.Stage,
		&convErr.Class,
		&convErr.Code,
		&convErr.Message,
		&detailsJSON,
		&convErr.Attempt,
		&convErr.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest error: %w", err)
	}

	if err := json.Unmarshal(detailsJSON, &convErr.Details); err != nil {
		convErr.Details = make(map[string]any)
	}

	return &convErr, nil
}

// CountByClass counts errors by class
func (r *ErrorRepository) CountByClass(ctx context.Context) (map[domain.ErrorClass]int, error) {
	query := `SELECT class, COUNT(*) FROM conversion_errors GROUP BY class`

	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to count errors: %w", err)
	}
	defer rows.Close()

	counts := make(map[domain.ErrorClass]int)
	for rows.Next() {
		var class domain.ErrorClass
		var count int
		if err := rows.Scan(&class, &count); err != nil {
			return nil, fmt.Errorf("failed to scan count: %w", err)
		}
		counts[class] = count
	}

	return counts, nil
}
