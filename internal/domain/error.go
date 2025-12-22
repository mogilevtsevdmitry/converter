package domain

import (
	"time"

	"github.com/google/uuid"
)

// ErrorClass represents error classification
type ErrorClass string

const (
	ErrorClassFatal     ErrorClass = "FATAL"
	ErrorClassRetryable ErrorClass = "RETRYABLE"
)

// ConversionError represents an error that occurred during conversion
type ConversionError struct {
	ID        uuid.UUID         `json:"id" db:"id"`
	JobID     uuid.UUID         `json:"jobId" db:"job_id"`
	Stage     Stage             `json:"stage" db:"stage"`
	Class     ErrorClass        `json:"class" db:"class"`
	Code      string            `json:"code" db:"code"`
	Message   string            `json:"message" db:"message"`
	Details   map[string]any    `json:"details" db:"details"`
	Attempt   int               `json:"attempt" db:"attempt"`
	CreatedAt time.Time         `json:"createdAt" db:"created_at"`
}

// NewConversionError creates a new conversion error
func NewConversionError(jobID uuid.UUID, stage Stage, class ErrorClass, code, message string, attempt int) *ConversionError {
	return &ConversionError{
		ID:        uuid.New(),
		JobID:     jobID,
		Stage:     stage,
		Class:     class,
		Code:      code,
		Message:   message,
		Details:   make(map[string]any),
		Attempt:   attempt,
		CreatedAt: time.Now().UTC(),
	}
}

// WithDetails adds details to the error
func (e *ConversionError) WithDetails(key string, value any) *ConversionError {
	e.Details[key] = value
	return e
}

// Error codes
const (
	ErrCodeUnsupportedFormat    = "UNSUPPORTED_FORMAT"
	ErrCodeInsufficientDisk     = "INSUFFICIENT_DISK"
	ErrCodeCorruptedFile        = "CORRUPTED_FILE"
	ErrCodeS3AccessDenied       = "S3_ACCESS_DENIED"
	ErrCodeS3NotFound           = "S3_NOT_FOUND"
	ErrCodeS3Timeout            = "S3_TIMEOUT"
	ErrCodeFFmpegFailed         = "FFMPEG_FAILED"
	ErrCodeFFprobeFailed        = "FFPROBE_FAILED"
	ErrCodeNetworkError         = "NETWORK_ERROR"
	ErrCodeInternalError        = "INTERNAL_ERROR"
	ErrCodeTimeout              = "TIMEOUT"
	ErrCodeCanceled             = "CANCELED"
)

// IsRetryable returns true if the error code is retryable
func IsRetryable(code string) bool {
	retryableCodes := map[string]bool{
		ErrCodeS3Timeout:     true,
		ErrCodeNetworkError:  true,
	}
	return retryableCodes[code]
}

// ClassifyError determines the error class based on error code
func ClassifyError(code string) ErrorClass {
	if IsRetryable(code) {
		return ErrorClassRetryable
	}
	return ErrorClassFatal
}
