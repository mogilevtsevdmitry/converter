package api

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"

	"github.com/tvoe/converter/internal/config"
	"github.com/tvoe/converter/internal/db"
	"github.com/tvoe/converter/internal/domain"
	"github.com/tvoe/converter/internal/metrics"
	"github.com/tvoe/converter/internal/storage/s3"
	"github.com/tvoe/converter/internal/temporal/workflows"
)

// Handler holds API dependencies
type Handler struct {
	config         *config.Config
	jobRepo        *db.JobRepository
	errorRepo      *db.ErrorRepository
	artifactRepo   *db.ArtifactRepository
	s3Client       *s3.Client
	temporalClient client.Client
	logger         *zap.Logger
	metrics        *metrics.Metrics
}

// NewHandler creates a new handler
func NewHandler(
	cfg *config.Config,
	jobRepo *db.JobRepository,
	errorRepo *db.ErrorRepository,
	artifactRepo *db.ArtifactRepository,
	s3Client *s3.Client,
	temporalClient client.Client,
	logger *zap.Logger,
	m *metrics.Metrics,
) *Handler {
	return &Handler{
		config:         cfg,
		jobRepo:        jobRepo,
		errorRepo:      errorRepo,
		artifactRepo:   artifactRepo,
		s3Client:       s3Client,
		temporalClient: temporalClient,
		logger:         logger,
		metrics:        m,
	}
}

// CreateJobRequest represents the request to create a job
type CreateJobRequest struct {
	Source         SourceConfig   `json:"source"`
	Profile        domain.Profile `json:"profile"`
	Priority       int            `json:"priority"`
	IdempotencyKey string         `json:"idempotencyKey,omitempty"`
	VideoID        *uuid.UUID     `json:"videoId,omitempty"`
}

// SourceConfig represents source configuration
type SourceConfig struct {
	Type   string `json:"type"`
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
}

// CreateJobResponse represents the response after creating a job
type CreateJobResponse struct {
	JobID     uuid.UUID        `json:"jobId"`
	Status    domain.JobStatus `json:"status"`
	CreatedAt time.Time        `json:"createdAt"`
}

// JobStatusResponse represents job status response
type JobStatusResponse struct {
	ID              uuid.UUID        `json:"id"`
	Status          domain.JobStatus `json:"status"`
	CurrentStage    *domain.Stage    `json:"currentStage,omitempty"`
	StageProgress   int              `json:"stageProgress"`
	OverallProgress int              `json:"overallProgress"`
	CreatedAt       time.Time        `json:"createdAt"`
	StartedAt       *time.Time       `json:"startedAt,omitempty"`
	UpdatedAt       time.Time        `json:"updatedAt"`
	FinishedAt      *time.Time       `json:"finishedAt,omitempty"`
	Errors          []*ErrorResponse `json:"errors,omitempty"`
}

// ErrorResponse represents error response
type ErrorResponse struct {
	Stage     domain.Stage      `json:"stage"`
	Class     domain.ErrorClass `json:"class"`
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	CreatedAt time.Time         `json:"createdAt"`
}

// ArtifactResponse represents artifact response
type ArtifactResponse struct {
	ID        uuid.UUID           `json:"id"`
	Type      domain.ArtifactType `json:"type"`
	Bucket    string              `json:"bucket"`
	Key       string              `json:"key"`
	SizeBytes *int64              `json:"sizeBytes,omitempty"`
	CreatedAt time.Time           `json:"createdAt"`
}

// DRMKeyResponse represents DRM key response for testing/development
type DRMKeyResponse struct {
	KeyID    string `json:"keyId"`
	Key      string `json:"key,omitempty"` // Only returned in dev mode
	Provider string `json:"provider"`
	LAURL    string `json:"laUrl,omitempty"`   // License Acquisition URL
	CertURL  string `json:"certUrl,omitempty"` // Certificate URL (FairPlay)
}

// CreateJob creates a new conversion job
func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
	var req CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate request
	if req.Source.Type != "s3" {
		h.writeError(w, http.StatusBadRequest, "only s3 source type is supported")
		return
	}
	if req.Source.Bucket == "" || req.Source.Key == "" {
		h.writeError(w, http.StatusBadRequest, "source bucket and key are required")
		return
	}

	ctx := r.Context()

	// Check idempotency
	if req.IdempotencyKey != "" {
		existingJob, err := h.jobRepo.GetByIdempotencyKey(ctx, req.IdempotencyKey)
		if err == nil && existingJob != nil {
			h.writeJSON(w, http.StatusOK, CreateJobResponse{
				JobID:     existingJob.ID,
				Status:    existingJob.Status,
				CreatedAt: existingJob.CreatedAt,
			})
			return
		}
	}

	// Set default profile values
	if len(req.Profile.Qualities) == 0 {
		req.Profile = domain.DefaultProfile()
	}

	// Create job
	job := domain.NewJob(req.Source.Bucket, req.Source.Key, req.Profile)
	job.Priority = req.Priority
	job.VideoID = req.VideoID
	if req.IdempotencyKey != "" {
		job.IdempotencyKey = &req.IdempotencyKey
	}

	if err := h.jobRepo.Create(ctx, job); err != nil {
		h.logger.Error("failed to create job", zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "failed to create job")
		return
	}

	// Start Temporal workflow
	workflowID := "video-conversion-" + job.ID.String()
	workflowOptions := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: h.config.Temporal.TaskQueue,
	}

	workflowRun, err := h.temporalClient.ExecuteWorkflow(ctx, workflowOptions, workflows.VideoConversionWorkflow, workflows.VideoConversionWorkflowInput{
		JobID: job.ID,
	})
	if err != nil {
		h.logger.Error("failed to start workflow", zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "failed to start workflow")
		return
	}

	// Update job with workflow ID
	if err := h.jobRepo.SetWorkflowID(ctx, job.ID, workflowRun.GetID()); err != nil {
		h.logger.Error("failed to set workflow ID", zap.Error(err))
	}

	h.metrics.IncrementJobsTotal(string(domain.JobStatusQueued))
	h.logger.Info("job created",
		zap.String("jobId", job.ID.String()),
		zap.String("workflowId", workflowRun.GetID()),
	)

	h.writeJSON(w, http.StatusCreated, CreateJobResponse{
		JobID:     job.ID,
		Status:    job.Status,
		CreatedAt: job.CreatedAt,
	})
}

// GetJob gets job status
func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	jobIDStr := chi.URLParam(r, "jobId")
	jobID, err := uuid.Parse(jobIDStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid job ID")
		return
	}

	ctx := r.Context()

	job, err := h.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			h.writeError(w, http.StatusNotFound, "job not found")
			return
		}
		h.logger.Error("failed to get job", zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "failed to get job")
		return
	}

	response := JobStatusResponse{
		ID:              job.ID,
		Status:          job.Status,
		CurrentStage:    job.CurrentStage,
		StageProgress:   job.StageProgress,
		OverallProgress: job.OverallProgress,
		CreatedAt:       job.CreatedAt,
		StartedAt:       job.StartedAt,
		UpdatedAt:       job.UpdatedAt,
		FinishedAt:      job.FinishedAt,
	}

	// Get errors if job failed
	if job.Status == domain.JobStatusFailed {
		errors, err := h.errorRepo.GetByJobID(ctx, jobID)
		if err == nil {
			for _, e := range errors {
				response.Errors = append(response.Errors, &ErrorResponse{
					Stage:     e.Stage,
					Class:     e.Class,
					Code:      e.Code,
					Message:   e.Message,
					CreatedAt: e.CreatedAt,
				})
			}
		}
	}

	h.writeJSON(w, http.StatusOK, response)
}

// CancelJob cancels a job
func (h *Handler) CancelJob(w http.ResponseWriter, r *http.Request) {
	jobIDStr := chi.URLParam(r, "jobId")
	jobID, err := uuid.Parse(jobIDStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid job ID")
		return
	}

	ctx := r.Context()

	job, err := h.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			h.writeError(w, http.StatusNotFound, "job not found")
			return
		}
		h.logger.Error("failed to get job", zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "failed to get job")
		return
	}

	// Check if job can be cancelled
	if job.Status == domain.JobStatusCompleted || job.Status == domain.JobStatusCanceled {
		h.writeError(w, http.StatusBadRequest, "job cannot be cancelled")
		return
	}

	// Cancel Temporal workflow
	if job.WorkflowID != nil {
		err := h.temporalClient.SignalWorkflow(ctx, *job.WorkflowID, "", "cancel", nil)
		if err != nil {
			h.logger.Error("failed to signal workflow", zap.Error(err))
		}

		// Also try to cancel the workflow
		err = h.temporalClient.CancelWorkflow(ctx, *job.WorkflowID, "")
		if err != nil {
			h.logger.Error("failed to cancel workflow", zap.Error(err))
		}
	}

	// Update job status
	if err := h.jobRepo.SetFinished(ctx, jobID, domain.JobStatusCanceled); err != nil {
		h.logger.Error("failed to update job status", zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "failed to cancel job")
		return
	}

	h.metrics.IncrementJobsTotal(string(domain.JobStatusCanceled))
	h.logger.Info("job cancelled", zap.String("jobId", jobID.String()))

	h.writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// GetArtifacts gets job artifacts
func (h *Handler) GetArtifacts(w http.ResponseWriter, r *http.Request) {
	jobIDStr := chi.URLParam(r, "jobId")
	jobID, err := uuid.Parse(jobIDStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid job ID")
		return
	}

	ctx := r.Context()

	artifacts, err := h.artifactRepo.GetByJobID(ctx, jobID)
	if err != nil {
		h.logger.Error("failed to get artifacts", zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "failed to get artifacts")
		return
	}

	response := make([]*ArtifactResponse, 0, len(artifacts))
	for _, a := range artifacts {
		response = append(response, &ArtifactResponse{
			ID:        a.ID,
			Type:      a.Type,
			Bucket:    a.Bucket,
			Key:       a.Key,
			SizeBytes: a.SizeBytes,
			CreatedAt: a.CreatedAt,
		})
	}

	h.writeJSON(w, http.StatusOK, response)
}

// HealthCheck returns health status
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	status := map[string]string{
		"status": "healthy",
	}

	// Check database
	if _, err := h.jobRepo.GetByID(ctx, uuid.Nil); err != nil && !errors.Is(err, db.ErrNotFound) {
		h.logger.Error("database health check failed", zap.Error(err))
		status["database"] = "unhealthy"
		status["status"] = "unhealthy"
	} else {
		status["database"] = "healthy"
	}

	// Check S3
	if err := h.s3Client.Health(ctx); err != nil {
		h.logger.Error("S3 health check failed", zap.Error(err))
		status["s3"] = "unhealthy"
		status["status"] = "unhealthy"
	} else {
		status["s3"] = "healthy"
	}

	// Check Temporal
	// Temporal health is implied by successful workflow operations

	statusCode := http.StatusOK
	if status["status"] == "unhealthy" {
		statusCode = http.StatusServiceUnavailable
	}

	h.writeJSON(w, statusCode, status)
}

// ReadyCheck returns readiness status
func (h *Handler) ReadyCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	status := map[string]string{
		"status": "ready",
	}

	// Check database connection
	if _, err := h.jobRepo.GetByID(ctx, uuid.Nil); err != nil && !errors.Is(err, db.ErrNotFound) {
		status["status"] = "not ready"
		status["database"] = "not connected"
	}

	// Check S3
	if err := h.s3Client.Health(ctx); err != nil {
		status["status"] = "not ready"
		status["s3"] = "not connected"
	}

	statusCode := http.StatusOK
	if status["status"] != "ready" {
		statusCode = http.StatusServiceUnavailable
	}

	h.writeJSON(w, statusCode, status)
}

// GetDRMKey returns DRM key information for a job (development/testing only)
func (h *Handler) GetDRMKey(w http.ResponseWriter, r *http.Request) {
	jobIDStr := chi.URLParam(r, "jobId")
	jobID, err := uuid.Parse(jobIDStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid job ID")
		return
	}

	ctx := r.Context()

	// Verify job exists
	job, err := h.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			h.writeError(w, http.StatusNotFound, "job not found")
			return
		}
		h.logger.Error("failed to get job", zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "failed to get job")
		return
	}

	// Check if DRM is enabled
	if !h.config.DRM.Enabled {
		h.writeError(w, http.StatusNotFound, "DRM is not enabled")
		return
	}

	// Build response based on DRM provider
	response := DRMKeyResponse{
		Provider: h.config.DRM.Provider,
	}

	// Get key ID based on provider
	switch h.config.DRM.Provider {
	case "widevine":
		response.KeyID = h.config.DRM.WidevineKeyID
		response.LAURL = h.config.DRM.KeyServerURL
	case "fairplay":
		response.KeyID = h.config.DRM.WidevineKeyID // FairPlay uses same key ID format
		response.CertURL = h.config.DRM.SignerURL
		response.LAURL = h.config.DRM.FairPlayKeyURL
	case "playready":
		response.KeyID = h.config.DRM.PlayReadyKeyID
		response.LAURL = h.config.DRM.PlayReadyLAURL
	default:
		response.KeyID = h.config.DRM.WidevineKeyID
		if response.KeyID == "" {
			response.KeyID = h.config.DRM.PlayReadyKeyID
		}
	}

	// In development mode (no production key server), include the actual key
	// WARNING: Never do this in production!
	if h.config.DRM.KeyServerURL == "" {
		switch h.config.DRM.Provider {
		case "widevine":
			response.Key = h.config.DRM.WidevineKey
		case "playready":
			response.Key = h.config.DRM.PlayReadyKey
		default:
			response.Key = h.config.DRM.WidevineKey
			if response.Key == "" {
				response.Key = h.config.DRM.PlayReadyKey
			}
		}
	}

	h.logger.Info("DRM key requested",
		zap.String("jobId", job.ID.String()),
		zap.String("provider", response.Provider),
	)

	h.writeJSON(w, http.StatusOK, response)
}

// ServeDRMKeyFile serves the raw encryption key file (for HLS AES-128)
func (h *Handler) ServeDRMKeyFile(w http.ResponseWriter, r *http.Request) {
	jobIDStr := chi.URLParam(r, "jobId")
	_, err := uuid.Parse(jobIDStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid job ID")
		return
	}

	// For HLS AES-128 encryption, serve the raw key
	// In production, this should be protected by authentication
	if !h.config.HLS.EnableEncryption && !h.config.DRM.Enabled {
		h.writeError(w, http.StatusNotFound, "encryption is not enabled")
		return
	}

	// Get key from S3 or generate based on job ID
	// For now, return the configured key or a derived key
	var keyBytes []byte
	if h.config.DRM.WidevineKey != "" {
		// Decode hex key
		keyBytes = make([]byte, 16)
		_, err := hex.Decode(keyBytes, []byte(h.config.DRM.WidevineKey))
		if err != nil {
			h.logger.Error("failed to decode key", zap.Error(err))
			h.writeError(w, http.StatusInternalServerError, "invalid key configuration")
			return
		}
	} else {
		// Return 404 - key not configured
		h.writeError(w, http.StatusNotFound, "key not found")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", "16")
	w.WriteHeader(http.StatusOK)
	w.Write(keyBytes)
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, map[string]string{"error": message})
}
