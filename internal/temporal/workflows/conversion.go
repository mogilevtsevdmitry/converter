package workflows

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/tvoe/converter/internal/domain"
	"github.com/tvoe/converter/internal/temporal/activities"
)

// VideoConversionWorkflowInput holds workflow input
type VideoConversionWorkflowInput struct {
	JobID uuid.UUID `json:"jobId"`
}

// VideoConversionWorkflowOutput holds workflow output
type VideoConversionWorkflowOutput struct {
	Status        domain.JobStatus `json:"status"`
	ArtifactCount int             `json:"artifactCount"`
	Error         string          `json:"error,omitempty"`
}

// VideoConversionWorkflow orchestrates the video conversion process
func VideoConversionWorkflow(ctx workflow.Context, input VideoConversionWorkflowInput) (*VideoConversionWorkflowOutput, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting video conversion workflow", "jobId", input.JobID.String())

	// Set up activity options with retry policy
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 6 * time.Hour,
		HeartbeatTimeout:    1 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	// Ensure job status is updated on workflow completion (success or failure)
	output := &VideoConversionWorkflowOutput{
		Status: domain.JobStatusRunning,
	}
	defer func() {
		// Use disconnected context for finalization to ensure it runs even if workflow is cancelled
		finalizeCtx, _ := workflow.NewDisconnectedContext(ctx)
		finalizeOptions := workflow.ActivityOptions{
			StartToCloseTimeout: 1 * time.Minute,
			RetryPolicy: &temporal.RetryPolicy{
				InitialInterval:    time.Second,
				BackoffCoefficient: 2.0,
				MaximumInterval:    10 * time.Second,
				MaximumAttempts:    5,
			},
		}
		finalizeCtx = workflow.WithActivityOptions(finalizeCtx, finalizeOptions)

		_ = workflow.ExecuteActivity(finalizeCtx, "FinalizeJob", activities.FinalizeJobInput{
			JobID:  input.JobID,
			Status: output.Status,
			Error:  output.Error,
		}).Get(finalizeCtx, nil)
	}()

	// Set up signal channel for cancellation
	cancelChan := workflow.GetSignalChannel(ctx, "cancel")

	// Create selector for handling signals
	selector := workflow.NewSelector(ctx)

	var cancelled bool
	selector.AddReceive(cancelChan, func(c workflow.ReceiveChannel, more bool) {
		cancelled = true
		logger.Info("Received cancel signal")
	})

	// Helper to check cancellation
	checkCancelled := func() bool {
		// Non-blocking check
		for selector.HasPending() {
			selector.Select(ctx)
		}
		return cancelled
	}

	// Step 1: Extract Metadata
	logger.Info("Starting metadata extraction")
	var metadataOutput *activities.MetadataOutput
	err := workflow.ExecuteActivity(ctx, "ExtractMetadata", activities.ActivityInput{JobID: input.JobID}).Get(ctx, &metadataOutput)
	if err != nil {
		output.Status = domain.JobStatusFailed
		output.Error = fmt.Sprintf("metadata extraction failed: %v", err)
		return output, err
	}

	if checkCancelled() {
		return handleCancellation(ctx, input.JobID, output)
	}

	// Step 2: Validate Inputs
	logger.Info("Starting validation")
	err = workflow.ExecuteActivity(ctx, "ValidateInputs", activities.ValidationInput{
		JobID:    input.JobID,
		Metadata: metadataOutput.Metadata,
	}).Get(ctx, nil)
	if err != nil {
		output.Status = domain.JobStatusFailed
		output.Error = fmt.Sprintf("validation failed: %v", err)
		return output, err
	}

	if checkCancelled() {
		return handleCancellation(ctx, input.JobID, output)
	}

	// Step 3: Transcode
	logger.Info("Starting transcoding")
	transcodeOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 12 * time.Hour,
		HeartbeatTimeout:    5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    10 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    5 * time.Minute,
			MaximumAttempts:    2,
		},
	}
	transcodeCtx := workflow.WithActivityOptions(ctx, transcodeOptions)

	var transcodeOutput *activities.TranscodeOutput
	err = workflow.ExecuteActivity(transcodeCtx, "Transcode", activities.TranscodeInput{
		JobID:    input.JobID,
		Metadata: metadataOutput.Metadata,
	}).Get(ctx, &transcodeOutput)
	if err != nil {
		output.Status = domain.JobStatusFailed
		output.Error = fmt.Sprintf("transcoding failed: %v", err)
		return output, err
	}

	if checkCancelled() {
		return handleCancellation(ctx, input.JobID, output)
	}

	// Step 4: Extract Subtitles (optional, non-blocking)
	logger.Info("Starting subtitle extraction")
	var subtitlesOutput *activities.SubtitlesOutput
	err = workflow.ExecuteActivity(ctx, "ExtractSubtitles", activities.SubtitlesInput{
		JobID:    input.JobID,
		Metadata: metadataOutput.Metadata,
	}).Get(ctx, &subtitlesOutput)
	if err != nil {
		// Log but don't fail - subtitles are optional
		logger.Warn("Subtitle extraction failed", "error", err)
	}

	if checkCancelled() {
		return handleCancellation(ctx, input.JobID, output)
	}

	// Step 5: Generate Thumbnails
	logger.Info("Starting thumbnail generation")
	var thumbnailsOutput *activities.ThumbnailsOutput
	err = workflow.ExecuteActivity(ctx, "GenerateThumbnails", activities.ThumbnailsInput{
		JobID:    input.JobID,
		Metadata: metadataOutput.Metadata,
	}).Get(ctx, &thumbnailsOutput)
	if err != nil {
		// Log but don't fail - thumbnails are optional
		logger.Warn("Thumbnail generation failed", "error", err)
	}

	if checkCancelled() {
		return handleCancellation(ctx, input.JobID, output)
	}

	// Step 6: HLS Segmentation (and DASH manifest generation for fMP4)
	logger.Info("Starting HLS segmentation")
	var hlsOutput *activities.HLSOutput
	err = workflow.ExecuteActivity(ctx, "SegmentHLS", activities.HLSInput{
		JobID:           input.JobID,
		OutputPaths:     transcodeOutput.OutputPaths,
		TierOutputPaths: transcodeOutput.TierOutputPaths,
		EnabledTiers:    transcodeOutput.EnabledTiers,
		Duration:        metadataOutput.Metadata.Duration,
	}).Get(ctx, &hlsOutput)
	if err != nil {
		output.Status = domain.JobStatusFailed
		output.Error = fmt.Sprintf("HLS segmentation failed: %v", err)
		return output, err
	}

	if checkCancelled() {
		return handleCancellation(ctx, input.JobID, output)
	}

	// Step 7: Upload Artifacts
	logger.Info("Starting artifact upload")
	uploadOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Hour,
		HeartbeatTimeout:    1 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    5 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    2 * time.Minute,
			MaximumAttempts:    5,
		},
	}
	uploadCtx := workflow.WithActivityOptions(ctx, uploadOptions)

	var uploadOutput *activities.UploadOutput
	err = workflow.ExecuteActivity(uploadCtx, "UploadArtifacts", activities.UploadInput{
		JobID: input.JobID,
	}).Get(ctx, &uploadOutput)
	if err != nil {
		output.Status = domain.JobStatusFailed
		output.Error = fmt.Sprintf("upload failed: %v", err)
		return output, err
	}

	// Step 8: Cleanup
	logger.Info("Starting cleanup")
	cleanupOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    3,
		},
	}
	cleanupCtx := workflow.WithActivityOptions(ctx, cleanupOptions)

	err = workflow.ExecuteActivity(cleanupCtx, "Cleanup", activities.CleanupInput{
		JobID: input.JobID,
	}).Get(ctx, nil)
	if err != nil {
		// Log but don't fail - cleanup is best effort
		logger.Warn("Cleanup failed", "error", err)
	}

	output.Status = domain.JobStatusCompleted
	output.ArtifactCount = uploadOutput.ArtifactCount
	logger.Info("Video conversion workflow completed successfully",
		"jobId", input.JobID.String(),
		"artifactCount", output.ArtifactCount)

	return output, nil
}

// handleCancellation handles workflow cancellation
func handleCancellation(ctx workflow.Context, jobID uuid.UUID, output *VideoConversionWorkflowOutput) (*VideoConversionWorkflowOutput, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Handling cancellation", "jobId", jobID.String())

	// Create disconnected context for cleanup
	cleanupCtx, _ := workflow.NewDisconnectedContext(ctx)
	cleanupOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 1,
		},
	}
	cleanupCtx = workflow.WithActivityOptions(cleanupCtx, cleanupOptions)

	// Run cleanup
	_ = workflow.ExecuteActivity(cleanupCtx, "Cleanup", activities.CleanupInput{
		JobID: jobID,
	}).Get(cleanupCtx, nil)

	output.Status = domain.JobStatusCanceled
	output.Error = "workflow cancelled by user"
	return output, nil
}
