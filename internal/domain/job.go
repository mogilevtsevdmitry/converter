package domain

import (
	"time"

	"github.com/google/uuid"
)

// JobStatus represents the status of a conversion job
type JobStatus string

const (
	JobStatusQueued    JobStatus = "QUEUED"
	JobStatusRunning   JobStatus = "RUNNING"
	JobStatusCompleted JobStatus = "COMPLETED"
	JobStatusFailed    JobStatus = "FAILED"
	JobStatusCanceled  JobStatus = "CANCELED"
)

// Stage represents a conversion stage
type Stage string

const (
	StageMetadataExtraction  Stage = "METADATA_EXTRACTION"
	StageValidation          Stage = "VALIDATION"
	StageTranscoding         Stage = "TRANSCODING"
	StageSubtitlesExtraction Stage = "SUBTITLES_EXTRACTION"
	StageThumbnailsGen       Stage = "THUMBNAILS_GENERATION"
	StageHLSSegmentation     Stage = "HLS_SEGMENTATION"
	StageUploading           Stage = "UPLOADING"
	StageCleanup             Stage = "CLEANUP"
)

// AllStages returns ordered list of all stages
func AllStages() []Stage {
	return []Stage{
		StageMetadataExtraction,
		StageValidation,
		StageTranscoding,
		StageSubtitlesExtraction,
		StageThumbnailsGen,
		StageHLSSegmentation,
		StageUploading,
		StageCleanup,
	}
}

// StageWeight returns the weight of a stage for overall progress calculation
func StageWeight(s Stage) int {
	weights := map[Stage]int{
		StageMetadataExtraction:  5,
		StageValidation:          5,
		StageTranscoding:         50,
		StageSubtitlesExtraction: 5,
		StageThumbnailsGen:       10,
		StageHLSSegmentation:     10,
		StageUploading:           10,
		StageCleanup:             5,
	}
	return weights[s]
}

// Job represents a video conversion job
type Job struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	VideoID         *uuid.UUID `json:"videoId,omitempty" db:"video_id"`
	SourceBucket    string     `json:"sourceBucket" db:"source_bucket"`
	SourceKey       string     `json:"sourceKey" db:"source_key"`
	Status          JobStatus  `json:"status" db:"status"`
	CurrentStage    *Stage     `json:"currentStage,omitempty" db:"current_stage"`
	StageProgress   int        `json:"stageProgress" db:"stage_progress"`
	OverallProgress int        `json:"overallProgress" db:"overall_progress"`
	Profile         Profile    `json:"profile" db:"profile"`
	IdempotencyKey  *string    `json:"idempotencyKey,omitempty" db:"idempotency_key"`
	WorkflowID      *string    `json:"workflowId,omitempty" db:"workflow_id"`
	Priority        int        `json:"priority" db:"priority"`
	CreatedAt       time.Time  `json:"createdAt" db:"created_at"`
	StartedAt       *time.Time `json:"startedAt,omitempty" db:"started_at"`
	UpdatedAt       time.Time  `json:"updatedAt" db:"updated_at"`
	FinishedAt      *time.Time `json:"finishedAt,omitempty" db:"finished_at"`
	Attempt         int        `json:"attempt" db:"attempt"`
	LastErrorID     *uuid.UUID `json:"lastErrorId,omitempty" db:"last_error_id"`
	LockVersion     int        `json:"-" db:"lock_version"`
}

// NewJob creates a new job with default values
func NewJob(sourceBucket, sourceKey string, profile Profile) *Job {
	now := time.Now().UTC()
	return &Job{
		ID:              uuid.New(),
		SourceBucket:    sourceBucket,
		SourceKey:       sourceKey,
		Status:          JobStatusQueued,
		StageProgress:   0,
		OverallProgress: 0,
		Profile:         profile,
		Priority:        0,
		CreatedAt:       now,
		UpdatedAt:       now,
		Attempt:         0,
		LockVersion:     0,
	}
}

// CalculateOverallProgress calculates overall progress based on current stage and stage progress
func (j *Job) CalculateOverallProgress() int {
	if j.CurrentStage == nil {
		return 0
	}

	stages := AllStages()
	var completedWeight int
	var currentStageWeight int

	for _, s := range stages {
		if s == *j.CurrentStage {
			currentStageWeight = StageWeight(s)
			break
		}
		completedWeight += StageWeight(s)
	}

	totalWeight := 0
	for _, s := range stages {
		totalWeight += StageWeight(s)
	}

	progress := completedWeight + (currentStageWeight * j.StageProgress / 100)
	return progress * 100 / totalWeight
}
