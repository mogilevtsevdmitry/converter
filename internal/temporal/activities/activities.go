package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/zap"

	"github.com/tvoe/converter/internal/config"
	"github.com/tvoe/converter/internal/db"
	"github.com/tvoe/converter/internal/domain"
	"github.com/tvoe/converter/internal/drm"
	"github.com/tvoe/converter/internal/ffmpeg"
	"github.com/tvoe/converter/internal/metrics"
	"github.com/tvoe/converter/internal/storage/s3"
)

// Activities holds all activity implementations
type Activities struct {
	config      *config.Config
	jobRepo     *db.JobRepository
	errorRepo   *db.ErrorRepository
	artifactRepo *db.ArtifactRepository
	s3Client    *s3.Client
	logger      *zap.Logger
	metrics     *metrics.Metrics
}

// NewActivities creates a new activities instance
func NewActivities(
	cfg *config.Config,
	jobRepo *db.JobRepository,
	errorRepo *db.ErrorRepository,
	artifactRepo *db.ArtifactRepository,
	s3Client *s3.Client,
	logger *zap.Logger,
	m *metrics.Metrics,
) *Activities {
	return &Activities{
		config:       cfg,
		jobRepo:      jobRepo,
		errorRepo:    errorRepo,
		artifactRepo: artifactRepo,
		s3Client:     s3Client,
		logger:       logger,
		metrics:      m,
	}
}

// ActivityInput holds common input for activities
type ActivityInput struct {
	JobID uuid.UUID `json:"jobId"`
}

// MetadataOutput holds metadata extraction output
type MetadataOutput struct {
	Metadata *domain.VideoMetadata `json:"metadata"`
}

// ExtractMetadata extracts video metadata
func (a *Activities) ExtractMetadata(ctx context.Context, input ActivityInput) (*MetadataOutput, error) {
	logger := a.logger.With(zap.String("jobId", input.JobID.String()), zap.String("activity", "ExtractMetadata"))
	startTime := time.Now()
	defer func() {
		a.metrics.RecordStageDuration(string(domain.StageMetadataExtraction), time.Since(startTime).Seconds())
	}()

	// Update job status to RUNNING
	if err := a.jobRepo.UpdateStatus(ctx, input.JobID, domain.JobStatusRunning); err != nil {
		logger.Error("failed to update job status", zap.Error(err))
	}
	a.metrics.IncrementJobsActive()

	// Update progress
	if err := a.updateProgress(ctx, input.JobID, domain.StageMetadataExtraction, 0); err != nil {
		logger.Error("failed to update progress", zap.Error(err))
	}

	// Get job
	job, err := a.jobRepo.GetByID(ctx, input.JobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	// Create workspace
	workspace := ffmpeg.NewWorkspace(a.config.Worker.WorkdirRoot, input.JobID)
	if err := workspace.Create(); err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}

	// Download source file with periodic heartbeat
	inputPath := workspace.InputPath("source" + filepath.Ext(job.SourceKey))
	stopHeartbeat := startPeriodicHeartbeat(ctx, 30*time.Second, "downloading source file")
	err = a.s3Client.Download(ctx, job.SourceBucket, job.SourceKey, inputPath)
	stopHeartbeat()
	if err != nil {
		return nil, a.recordError(ctx, input.JobID, domain.StageMetadataExtraction, domain.ErrCodeS3NotFound, err)
	}

	if err := a.updateProgress(ctx, input.JobID, domain.StageMetadataExtraction, 50); err != nil {
		logger.Error("failed to update progress", zap.Error(err))
	}
	activity.RecordHeartbeat(ctx, "probing file")

	// Probe file
	prober := ffmpeg.NewProber(a.config.FFmpeg.FFprobePath)
	metadata, err := prober.Probe(ctx, inputPath)
	if err != nil {
		return nil, a.recordError(ctx, input.JobID, domain.StageMetadataExtraction, domain.ErrCodeFFprobeFailed, err)
	}

	// Save metadata to file
	metaJSON, _ := json.MarshalIndent(metadata, "", "  ")
	os.WriteFile(workspace.MetaPath("metadata.json"), metaJSON, 0644)

	if err := a.updateProgress(ctx, input.JobID, domain.StageMetadataExtraction, 100); err != nil {
		logger.Error("failed to update progress", zap.Error(err))
	}

	logger.Info("metadata extracted",
		zap.Duration("duration", metadata.Duration),
		zap.Int("width", metadata.Width),
		zap.Int("height", metadata.Height),
		zap.String("videoCodec", metadata.VideoCodec),
	)

	return &MetadataOutput{Metadata: metadata}, nil
}

// ValidationInput holds validation input
type ValidationInput struct {
	JobID    uuid.UUID             `json:"jobId"`
	Metadata *domain.VideoMetadata `json:"metadata"`
}

// ValidateInputs validates input files and resources
func (a *Activities) ValidateInputs(ctx context.Context, input ValidationInput) error {
	logger := a.logger.With(zap.String("jobId", input.JobID.String()), zap.String("activity", "ValidateInputs"))
	startTime := time.Now()
	defer func() {
		a.metrics.RecordStageDuration(string(domain.StageValidation), time.Since(startTime).Seconds())
	}()

	if err := a.updateProgress(ctx, input.JobID, domain.StageValidation, 0); err != nil {
		logger.Error("failed to update progress", zap.Error(err))
	}

	// Validate container
	if !domain.IsContainerSupported(input.Metadata.Container) {
		return a.recordError(ctx, input.JobID, domain.StageValidation, domain.ErrCodeUnsupportedFormat,
			fmt.Errorf("unsupported container: %s", input.Metadata.Container))
	}

	// Validate video codec
	if !domain.IsVideoCodecSupported(input.Metadata.VideoCodec) {
		return a.recordError(ctx, input.JobID, domain.StageValidation, domain.ErrCodeUnsupportedFormat,
			fmt.Errorf("unsupported video codec: %s", input.Metadata.VideoCodec))
	}

	if err := a.updateProgress(ctx, input.JobID, domain.StageValidation, 50); err != nil {
		logger.Error("failed to update progress", zap.Error(err))
	}

	// Check disk space
	var stat syscall.Statfs_t
	if err := syscall.Statfs(a.config.Worker.WorkdirRoot, &stat); err == nil {
		freeSpace := stat.Bavail * uint64(stat.Bsize)
		requiredSpace := uint64(input.Metadata.FileSize) * 5 // Estimate 5x source size needed

		if freeSpace < requiredSpace {
			return a.recordError(ctx, input.JobID, domain.StageValidation, domain.ErrCodeInsufficientDisk,
				fmt.Errorf("insufficient disk space: %d bytes free, %d required", freeSpace, requiredSpace))
		}
	}

	// Validate S3 access
	if err := a.s3Client.Health(ctx); err != nil {
		return a.recordError(ctx, input.JobID, domain.StageValidation, domain.ErrCodeS3AccessDenied, err)
	}

	if err := a.updateProgress(ctx, input.JobID, domain.StageValidation, 100); err != nil {
		logger.Error("failed to update progress", zap.Error(err))
	}

	logger.Info("validation passed")
	return nil
}

// TranscodeInput holds transcode input
type TranscodeInput struct {
	JobID    uuid.UUID             `json:"jobId"`
	Metadata *domain.VideoMetadata `json:"metadata"`
}

// TranscodeOutput holds transcode output
type TranscodeOutput struct {
	OutputPaths map[domain.Quality]string `json:"outputPaths"`
	// Multi-tier output paths: tier -> quality -> path
	TierOutputPaths map[domain.EncodingTier]map[domain.Quality]string `json:"tierOutputPaths,omitempty"`
	// EnabledTiers lists which tiers were encoded
	EnabledTiers []domain.EncodingTier `json:"enabledTiers,omitempty"`
}

// Transcode transcodes video to target qualities
func (a *Activities) Transcode(ctx context.Context, input TranscodeInput) (*TranscodeOutput, error) {
	logger := a.logger.With(zap.String("jobId", input.JobID.String()), zap.String("activity", "Transcode"))
	startTime := time.Now()
	defer func() {
		a.metrics.RecordStageDuration(string(domain.StageTranscoding), time.Since(startTime).Seconds())
	}()

	if err := a.updateProgress(ctx, input.JobID, domain.StageTranscoding, 0); err != nil {
		logger.Error("failed to update progress", zap.Error(err))
	}

	a.metrics.IncrementFFmpegProcesses()
	defer a.metrics.DecrementFFmpegProcesses()

	// Get job
	job, err := a.jobRepo.GetByID(ctx, input.JobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	workspace := ffmpeg.NewWorkspace(a.config.Worker.WorkdirRoot, input.JobID)
	inputPath := workspace.InputPath("source" + filepath.Ext(job.SourceKey))

	// Filter qualities based on source resolution
	qualities := domain.FilterQualitiesForResolution(job.Profile.Qualities, input.Metadata.Height)

	builder := ffmpeg.NewCommandBuilder(a.config.FFmpeg.BinaryPath, a.config.Worker.EnableGPU, &a.config.Encoding)
	runner := ffmpeg.NewRunner(a.config.FFmpeg.BinaryPath, a.config.FFmpeg.ProcessTimeout)

	// Determine enabled tiers
	var enabledTiers []domain.EncodingTier
	if a.config.Encoding.EnableLegacyTier {
		enabledTiers = append(enabledTiers, domain.TierLegacy)
	}
	if a.config.Encoding.EnableModernTier {
		enabledTiers = append(enabledTiers, domain.TierModern)
	}

	// If no tiers enabled, default to legacy for backward compatibility
	if len(enabledTiers) == 0 {
		enabledTiers = []domain.EncodingTier{domain.TierLegacy}
	}

	logger.Info("multi-tier transcoding",
		zap.Int("tiers", len(enabledTiers)),
		zap.Int("qualities", len(qualities)),
		zap.Strings("enabledTiers", func() []string {
			s := make([]string, len(enabledTiers))
			for i, t := range enabledTiers {
				s[i] = string(t)
			}
			return s
		}()))

	tierOutputPaths := make(map[domain.EncodingTier]map[domain.Quality]string)
	outputPaths := make(map[domain.Quality]string) // Legacy compatibility

	totalTasks := len(enabledTiers) * len(qualities)
	currentTask := 0

	for _, tier := range enabledTiers {
		tierConfig := domain.GetTierConfig(tier)
		tierDir := filepath.Join(workspace.Paths().Transcoded, string(tier))

		// Create tier directory
		if err := os.MkdirAll(tierDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create tier directory: %w", err)
		}

		tierOutputPaths[tier] = make(map[domain.Quality]string)

		for _, quality := range qualities {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			logger.Info("transcoding",
				zap.String("tier", string(tier)),
				zap.String("quality", string(quality)),
				zap.String("videoCodec", string(tierConfig.VideoCodec)))

			cmd := builder.BuildTranscodeCommandForTier(inputPath, tierDir, quality, input.Metadata, job.Profile, tier)

			err := runner.Run(ctx, cmd.Args, func(progress ffmpeg.Progress) {
				percent := ffmpeg.CalculateProgress(progress.OutTime, input.Metadata.Duration)
				overallPercent := (currentTask*100 + percent) / totalTasks
				a.updateProgress(ctx, input.JobID, domain.StageTranscoding, overallPercent)
				activity.RecordHeartbeat(ctx, overallPercent)
			})

			if err != nil {
				return nil, a.recordError(ctx, input.JobID, domain.StageTranscoding, domain.ErrCodeFFmpegFailed,
					fmt.Errorf("tier=%s quality=%s: %w", tier, quality, err))
			}

			if err := ffmpeg.ValidateOutput(cmd.OutputPath); err != nil {
				return nil, a.recordError(ctx, input.JobID, domain.StageTranscoding, domain.ErrCodeFFmpegFailed, err)
			}

			tierOutputPaths[tier][quality] = cmd.OutputPath

			// For backward compatibility, use legacy tier paths as main output
			if tier == domain.TierLegacy {
				outputPaths[quality] = cmd.OutputPath
			}

			currentTask++
			logger.Info("quality transcoded",
				zap.String("tier", string(tier)),
				zap.String("quality", string(quality)),
				zap.String("output", cmd.OutputPath))
		}
	}

	// If only modern tier is enabled, use it as main output
	if len(outputPaths) == 0 && len(tierOutputPaths[domain.TierModern]) > 0 {
		outputPaths = tierOutputPaths[domain.TierModern]
	}

	if err := a.updateProgress(ctx, input.JobID, domain.StageTranscoding, 100); err != nil {
		logger.Error("failed to update progress", zap.Error(err))
	}

	return &TranscodeOutput{
		OutputPaths:     outputPaths,
		TierOutputPaths: tierOutputPaths,
		EnabledTiers:    enabledTiers,
	}, nil
}

// SubtitlesInput holds subtitles extraction input
type SubtitlesInput struct {
	JobID    uuid.UUID             `json:"jobId"`
	Metadata *domain.VideoMetadata `json:"metadata"`
	IntroDuration time.Duration    `json:"introDuration"`
}

// SubtitlesOutput holds subtitles extraction output
type SubtitlesOutput struct {
	SubtitlePaths map[string]string `json:"subtitlePaths"`
}

// ExtractSubtitles extracts subtitles from video
func (a *Activities) ExtractSubtitles(ctx context.Context, input SubtitlesInput) (*SubtitlesOutput, error) {
	logger := a.logger.With(zap.String("jobId", input.JobID.String()), zap.String("activity", "ExtractSubtitles"))
	startTime := time.Now()
	defer func() {
		a.metrics.RecordStageDuration(string(domain.StageSubtitlesExtraction), time.Since(startTime).Seconds())
	}()

	if err := a.updateProgress(ctx, input.JobID, domain.StageSubtitlesExtraction, 0); err != nil {
		logger.Error("failed to update progress", zap.Error(err))
	}

	if len(input.Metadata.SubtitleTracks) == 0 {
		logger.Info("no subtitles to extract")
		a.updateProgress(ctx, input.JobID, domain.StageSubtitlesExtraction, 100)
		return &SubtitlesOutput{SubtitlePaths: make(map[string]string)}, nil
	}

	job, err := a.jobRepo.GetByID(ctx, input.JobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	workspace := ffmpeg.NewWorkspace(a.config.Worker.WorkdirRoot, input.JobID)
	inputPath := workspace.InputPath("source" + filepath.Ext(job.SourceKey))

	builder := ffmpeg.NewCommandBuilder(a.config.FFmpeg.BinaryPath, a.config.Worker.EnableGPU, &a.config.Encoding)
	runner := ffmpeg.NewRunner(a.config.FFmpeg.BinaryPath, a.config.FFmpeg.ProcessTimeout)

	subtitlePaths := make(map[string]string)
	totalTracks := len(input.Metadata.SubtitleTracks)

	for i, track := range input.Metadata.SubtitleTracks {
		lang := track.Language
		if lang == "" || lang == "und" {
			lang = fmt.Sprintf("track%d", track.Index)
		}

		outputPath := workspace.SubtitlePath(lang)
		cmd := builder.BuildSubtitleExtractCommand(inputPath, outputPath, track.Index)

		if err := runner.Run(ctx, cmd.Args, nil); err != nil {
			logger.Warn("failed to extract subtitle", zap.String("language", lang), zap.Error(err))
			continue
		}

		// Shift timestamps if intro was added
		if input.IntroDuration > 0 {
			if err := shiftVTTTimestamps(outputPath, input.IntroDuration); err != nil {
				logger.Warn("failed to shift subtitle timestamps", zap.Error(err))
			}
		}

		subtitlePaths[lang] = outputPath

		progress := ((i + 1) * 100) / totalTracks
		a.updateProgress(ctx, input.JobID, domain.StageSubtitlesExtraction, progress)
		activity.RecordHeartbeat(ctx, progress)
	}

	a.updateProgress(ctx, input.JobID, domain.StageSubtitlesExtraction, 100)
	logger.Info("subtitles extracted", zap.Int("count", len(subtitlePaths)))

	return &SubtitlesOutput{SubtitlePaths: subtitlePaths}, nil
}

// ThumbnailsInput holds thumbnails generation input
type ThumbnailsInput struct {
	JobID    uuid.UUID             `json:"jobId"`
	Metadata *domain.VideoMetadata `json:"metadata"`
}

// ThumbnailsOutput holds thumbnails generation output
type ThumbnailsOutput struct {
	TilePaths []string `json:"tilePaths"`
	VTTPath   string   `json:"vttPath"`
}

// GenerateThumbnails generates video thumbnails
func (a *Activities) GenerateThumbnails(ctx context.Context, input ThumbnailsInput) (*ThumbnailsOutput, error) {
	logger := a.logger.With(zap.String("jobId", input.JobID.String()), zap.String("activity", "GenerateThumbnails"))
	startTime := time.Now()
	defer func() {
		a.metrics.RecordStageDuration(string(domain.StageThumbnailsGen), time.Since(startTime).Seconds())
	}()

	if err := a.updateProgress(ctx, input.JobID, domain.StageThumbnailsGen, 0); err != nil {
		logger.Error("failed to update progress", zap.Error(err))
	}

	job, err := a.jobRepo.GetByID(ctx, input.JobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	workspace := ffmpeg.NewWorkspace(a.config.Worker.WorkdirRoot, input.JobID)
	inputPath := workspace.InputPath("source" + filepath.Ext(job.SourceKey))

	thumbConfig := job.Profile.Thumbnails
	if thumbConfig.MaxFrames == 0 {
		thumbConfig.MaxFrames = a.config.Thumbnails.MaxFrames
	}
	if thumbConfig.TileX == 0 {
		thumbConfig.TileX = 5
	}
	if thumbConfig.TileY == 0 {
		thumbConfig.TileY = 5
	}
	if thumbConfig.Width == 0 {
		thumbConfig.Width = 160
	}
	if thumbConfig.Height == 0 {
		thumbConfig.Height = 90
	}

	// Calculate interval
	durationSec := input.Metadata.Duration.Seconds()
	interval := durationSec / float64(thumbConfig.MaxFrames)
	if interval < 1 {
		interval = 1
	}

	builder := ffmpeg.NewCommandBuilder(a.config.FFmpeg.BinaryPath, a.config.Worker.EnableGPU, &a.config.Encoding)
	runner := ffmpeg.NewRunner(a.config.FFmpeg.BinaryPath, a.config.FFmpeg.ProcessTimeout)

	// Generate thumbnails
	thumbPattern := filepath.Join(workspace.Paths().Thumbs, "thumb_%05d.jpg")
	thumbCmd := builder.BuildThumbnailCommand(inputPath, thumbPattern, interval, thumbConfig.Width, thumbConfig.Height)

	if err := runner.Run(ctx, thumbCmd.Args, func(p ffmpeg.Progress) {
		percent := ffmpeg.CalculateProgress(p.OutTime, input.Metadata.Duration) / 2
		a.updateProgress(ctx, input.JobID, domain.StageThumbnailsGen, percent)
		activity.RecordHeartbeat(ctx, percent)
	}); err != nil {
		return nil, a.recordError(ctx, input.JobID, domain.StageThumbnailsGen, domain.ErrCodeFFmpegFailed, err)
	}

	// Create tiles
	tilePaths, err := createThumbnailTiles(ctx, workspace.Paths().Thumbs, thumbConfig.TileX, thumbConfig.TileY, builder, runner)
	if err != nil {
		logger.Warn("failed to create tiles, using individual thumbnails", zap.Error(err))
	}

	a.updateProgress(ctx, input.JobID, domain.StageThumbnailsGen, 80)

	// Generate VTT manifest
	vttPath := filepath.Join(workspace.Paths().Thumbs, "thumbnails.vtt")
	if err := generateThumbnailVTT(vttPath, tilePaths, interval, thumbConfig.Width, thumbConfig.Height, thumbConfig.TileX, thumbConfig.TileY); err != nil {
		logger.Warn("failed to generate VTT manifest", zap.Error(err))
	}

	a.updateProgress(ctx, input.JobID, domain.StageThumbnailsGen, 100)
	logger.Info("thumbnails generated", zap.Int("tiles", len(tilePaths)))

	return &ThumbnailsOutput{
		TilePaths: tilePaths,
		VTTPath:   vttPath,
	}, nil
}

// HLSInput holds HLS segmentation input
type HLSInput struct {
	JobID       uuid.UUID                 `json:"jobId"`
	OutputPaths map[domain.Quality]string `json:"outputPaths"`
	// Multi-tier output paths: tier -> quality -> path
	TierOutputPaths map[domain.EncodingTier]map[domain.Quality]string `json:"tierOutputPaths,omitempty"`
	// EnabledTiers lists which tiers were encoded
	EnabledTiers []domain.EncodingTier `json:"enabledTiers,omitempty"`
}

// HLSOutput holds HLS segmentation output
type HLSOutput struct {
	MasterPlaylistPath string `json:"masterPlaylistPath"`
	MPDPath            string `json:"mpdPath,omitempty"` // DASH manifest for DRM
	HLSDir             string `json:"hlsDir"`
	Encrypted          bool   `json:"encrypted"`
	DRMEnabled         bool   `json:"drmEnabled"`
	DRMProvider        string `json:"drmProvider,omitempty"`
	KeyPath            string `json:"keyPath,omitempty"`
	KeyID              string `json:"keyId,omitempty"`
	// Multi-codec support
	MultiCodec   bool                  `json:"multiCodec"`
	EnabledTiers []domain.EncodingTier `json:"enabledTiers,omitempty"`
}

// SegmentHLS creates HLS segments
func (a *Activities) SegmentHLS(ctx context.Context, input HLSInput) (*HLSOutput, error) {
	logger := a.logger.With(zap.String("jobId", input.JobID.String()), zap.String("activity", "SegmentHLS"))
	startTime := time.Now()
	defer func() {
		a.metrics.RecordStageDuration(string(domain.StageHLSSegmentation), time.Since(startTime).Seconds())
	}()

	if err := a.updateProgress(ctx, input.JobID, domain.StageHLSSegmentation, 0); err != nil {
		logger.Error("failed to update progress", zap.Error(err))
	}

	job, err := a.jobRepo.GetByID(ctx, input.JobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	workspace := ffmpeg.NewWorkspace(a.config.Worker.WorkdirRoot, input.JobID)
	hlsDir := workspace.HLSPath()

	// Check if DRM is enabled and Shaka Packager is available
	if a.config.DRM.Enabled {
		packager := drm.NewPackager(&a.config.DRM)
		if packager.IsAvailable() {
			logger.Info("Using DRM packaging with Shaka Packager", zap.String("provider", a.config.DRM.Provider))
			return a.segmentHLSWithDRM(ctx, input, packager, hlsDir, logger)
		}
		logger.Warn("DRM enabled but Shaka Packager not available, falling back to FFmpeg")
	}

	// Standard FFmpeg HLS (with optional AES-128 encryption)
	return a.segmentHLSWithFFmpeg(ctx, input, job, hlsDir, logger)
}

// segmentHLSWithDRM uses Shaka Packager for DRM-protected content
func (a *Activities) segmentHLSWithDRM(
	ctx context.Context,
	input HLSInput,
	packager *drm.Packager,
	hlsDir string,
	logger *zap.Logger,
) (*HLSOutput, error) {
	a.updateProgress(ctx, input.JobID, domain.StageHLSSegmentation, 10)
	activity.RecordHeartbeat(ctx, "starting DRM packaging")

	result, err := packager.Package(ctx, input.OutputPaths, hlsDir, input.JobID)
	if err != nil {
		return nil, a.recordError(ctx, input.JobID, domain.StageHLSSegmentation, domain.ErrCodeFFmpegFailed,
			fmt.Errorf("DRM packaging failed: %w", err))
	}

	a.updateProgress(ctx, input.JobID, domain.StageHLSSegmentation, 100)
	logger.Info("DRM packaging complete",
		zap.String("masterPlaylist", result.MasterPlaylistPath),
		zap.String("mpdPath", result.MPDPath),
		zap.String("keyId", result.KeyID),
	)

	return &HLSOutput{
		MasterPlaylistPath: result.MasterPlaylistPath,
		MPDPath:            result.MPDPath,
		HLSDir:             result.OutputDir,
		DRMEnabled:         true,
		DRMProvider:        a.config.DRM.Provider,
		KeyID:              result.KeyID,
		Encrypted:          true,
	}, nil
}

// segmentHLSWithFFmpeg uses FFmpeg for HLS (with optional AES-128 encryption)
func (a *Activities) segmentHLSWithFFmpeg(
	ctx context.Context,
	input HLSInput,
	job *domain.Job,
	hlsDir string,
	logger *zap.Logger,
) (*HLSOutput, error) {
	segmentDuration := job.Profile.HLS.SegmentDurationSec
	if segmentDuration == 0 {
		segmentDuration = a.config.HLS.SegmentDurationSec
	}

	builder := ffmpeg.NewCommandBuilder(a.config.FFmpeg.BinaryPath, a.config.Worker.EnableGPU, &a.config.Encoding)
	runner := ffmpeg.NewRunner(a.config.FFmpeg.BinaryPath, a.config.FFmpeg.ProcessTimeout)

	// Generate encryption if enabled
	var encryption *ffmpeg.EncryptionInfo
	if a.config.HLS.EnableEncryption {
		var err error
		encryption, err = ffmpeg.GenerateEncryption(hlsDir, input.JobID, a.config.HLS.KeyURL)
		if err != nil {
			return nil, a.recordError(ctx, input.JobID, domain.StageHLSSegmentation, domain.ErrCodeFFmpegFailed,
				fmt.Errorf("failed to generate encryption: %w", err))
		}
		logger.Info("HLS AES-128 encryption enabled", zap.String("keyURL", encryption.KeyURL))
	}

	// Check if multi-tier is enabled
	isMultiTier := len(input.TierOutputPaths) > 0 && len(input.EnabledTiers) > 0

	if isMultiTier {
		return a.segmentHLSMultiTier(ctx, input, job, hlsDir, segmentDuration, builder, runner, encryption, logger)
	}

	// Legacy single-tier processing
	qualities := make([]domain.Quality, 0, len(input.OutputPaths))
	for q := range input.OutputPaths {
		qualities = append(qualities, q)
	}

	totalQualities := len(qualities)
	for i, quality := range qualities {
		inputPath := input.OutputPaths[quality]
		cmd := builder.BuildHLSCommandWithEncryption(inputPath, hlsDir, string(quality), segmentDuration, encryption)

		if err := runner.Run(ctx, cmd.Args, func(p ffmpeg.Progress) {
			activity.RecordHeartbeat(ctx, i)
		}); err != nil {
			return nil, a.recordError(ctx, input.JobID, domain.StageHLSSegmentation, domain.ErrCodeFFmpegFailed, err)
		}

		progress := ((i + 1) * 100) / totalQualities
		a.updateProgress(ctx, input.JobID, domain.StageHLSSegmentation, progress)
		activity.RecordHeartbeat(ctx, progress)
		logger.Info("HLS segmentation complete for quality", zap.String("quality", string(quality)))
	}

	// Generate master playlist
	masterContent := ffmpeg.GenerateMasterPlaylist(qualities, true)
	masterPath := filepath.Join(hlsDir, "master.m3u8")
	if err := os.WriteFile(masterPath, []byte(masterContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to write master playlist: %w", err)
	}

	a.updateProgress(ctx, input.JobID, domain.StageHLSSegmentation, 100)
	logger.Info("HLS segmentation complete", zap.String("masterPlaylist", masterPath), zap.Bool("encrypted", encryption != nil))

	output := &HLSOutput{
		MasterPlaylistPath: masterPath,
		HLSDir:             hlsDir,
		Encrypted:          encryption != nil,
	}
	if encryption != nil {
		output.KeyPath = encryption.KeyPath
	}

	return output, nil
}

// segmentHLSMultiTier segments HLS for multiple encoding tiers
func (a *Activities) segmentHLSMultiTier(
	ctx context.Context,
	input HLSInput,
	job *domain.Job,
	hlsDir string,
	segmentDuration int,
	builder *ffmpeg.CommandBuilder,
	runner *ffmpeg.Runner,
	encryption *ffmpeg.EncryptionInfo,
	logger *zap.Logger,
) (*HLSOutput, error) {
	logger.Info("multi-tier HLS segmentation",
		zap.Int("tiers", len(input.EnabledTiers)),
		zap.Strings("enabledTiers", func() []string {
			s := make([]string, len(input.EnabledTiers))
			for i, t := range input.EnabledTiers {
				s[i] = string(t)
			}
			return s
		}()))

	// Calculate total tasks
	totalTasks := 0
	for _, tier := range input.EnabledTiers {
		if paths, ok := input.TierOutputPaths[tier]; ok {
			totalTasks += len(paths)
		}
	}

	currentTask := 0
	var qualities []domain.Quality

	for _, tier := range input.EnabledTiers {
		tierPaths, ok := input.TierOutputPaths[tier]
		if !ok {
			continue
		}

		tierConfig := domain.GetTierConfig(tier)
		tierHLSDir := filepath.Join(hlsDir, string(tier))

		// Create tier HLS directory
		if err := os.MkdirAll(tierHLSDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create tier HLS directory: %w", err)
		}

		for quality, inputPath := range tierPaths {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			// Collect qualities for master playlist (use first tier)
			if tier == input.EnabledTiers[0] {
				qualities = append(qualities, quality)
			}

			logger.Info("HLS segmentation",
				zap.String("tier", string(tier)),
				zap.String("quality", string(quality)),
				zap.String("container", string(tierConfig.Container)))

			cmd := builder.BuildHLSCommandForTier(inputPath, tierHLSDir, string(quality), segmentDuration, tier, encryption)

			if err := runner.Run(ctx, cmd.Args, func(p ffmpeg.Progress) {
				activity.RecordHeartbeat(ctx, currentTask)
			}); err != nil {
				return nil, a.recordError(ctx, input.JobID, domain.StageHLSSegmentation, domain.ErrCodeFFmpegFailed,
					fmt.Errorf("tier=%s quality=%s: %w", tier, quality, err))
			}

			currentTask++
			progress := (currentTask * 100) / totalTasks
			a.updateProgress(ctx, input.JobID, domain.StageHLSSegmentation, progress)
			activity.RecordHeartbeat(ctx, progress)

			logger.Info("HLS segmentation complete",
				zap.String("tier", string(tier)),
				zap.String("quality", string(quality)))
		}
	}

	// Generate multi-codec master playlist
	masterContent := ffmpeg.GenerateMultiCodecMasterPlaylist(qualities, input.EnabledTiers, true)
	masterPath := filepath.Join(hlsDir, "master.m3u8")
	if err := os.WriteFile(masterPath, []byte(masterContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to write master playlist: %w", err)
	}

	a.updateProgress(ctx, input.JobID, domain.StageHLSSegmentation, 100)
	logger.Info("multi-tier HLS segmentation complete",
		zap.String("masterPlaylist", masterPath),
		zap.Int("tiers", len(input.EnabledTiers)),
		zap.Bool("encrypted", encryption != nil))

	output := &HLSOutput{
		MasterPlaylistPath: masterPath,
		HLSDir:             hlsDir,
		Encrypted:          encryption != nil,
		MultiCodec:         true,
		EnabledTiers:       input.EnabledTiers,
	}
	if encryption != nil {
		output.KeyPath = encryption.KeyPath
	}

	return output, nil
}

// UploadInput holds upload input
type UploadInput struct {
	JobID uuid.UUID `json:"jobId"`
}

// UploadOutput holds upload output
type UploadOutput struct {
	ArtifactCount int `json:"artifactCount"`
}

// UploadArtifacts uploads artifacts to S3
func (a *Activities) UploadArtifacts(ctx context.Context, input UploadInput) (*UploadOutput, error) {
	logger := a.logger.With(zap.String("jobId", input.JobID.String()), zap.String("activity", "UploadArtifacts"))
	startTime := time.Now()
	defer func() {
		a.metrics.RecordStageDuration(string(domain.StageUploading), time.Since(startTime).Seconds())
	}()

	if err := a.updateProgress(ctx, input.JobID, domain.StageUploading, 0); err != nil {
		logger.Error("failed to update progress", zap.Error(err))
	}

	job, err := a.jobRepo.GetByID(ctx, input.JobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	workspace := ffmpeg.NewWorkspace(a.config.Worker.WorkdirRoot, input.JobID)
	bucket := a.s3Client.GetDefaultBucket()

	// Build S3 prefix
	videoID := input.JobID.String()
	if job.VideoID != nil {
		videoID = job.VideoID.String()
	}
	prefix := fmt.Sprintf("%s/%s", videoID, input.JobID.String())

	uploader := s3.NewDirectoryUploader(a.s3Client, a.config.Worker.MaxParallelUploads)

	var allArtifacts []*domain.Artifact

	// Upload HLS
	hlsArtifacts, err := uploader.UploadDirectory(ctx, input.JobID, workspace.HLSPath(), bucket, prefix+"/hls", func(p s3.UploadProgress) {
		progress := p.CompletedFiles * 50 / p.TotalFiles
		a.updateProgress(ctx, input.JobID, domain.StageUploading, progress)
		a.metrics.AddUploadBytes(float64(p.UploadedBytes))
		activity.RecordHeartbeat(ctx, progress)
	})
	if err != nil {
		return nil, a.recordError(ctx, input.JobID, domain.StageUploading, domain.ErrCodeNetworkError, err)
	}
	allArtifacts = append(allArtifacts, hlsArtifacts...)

	// Upload thumbnails
	thumbsArtifacts, err := uploader.UploadDirectory(ctx, input.JobID, workspace.Paths().Thumbs, bucket, prefix+"/thumbs", func(p s3.UploadProgress) {
		progress := 50 + p.CompletedFiles*30/p.TotalFiles
		a.updateProgress(ctx, input.JobID, domain.StageUploading, progress)
		activity.RecordHeartbeat(ctx, progress)
	})
	if err != nil {
		logger.Warn("failed to upload thumbnails", zap.Error(err))
	} else {
		allArtifacts = append(allArtifacts, thumbsArtifacts...)
	}

	// Upload subtitles
	subsArtifacts, err := uploader.UploadDirectory(ctx, input.JobID, workspace.Paths().Subtitles, bucket, prefix+"/subtitles", func(p s3.UploadProgress) {
		progress := 80 + p.CompletedFiles*10/p.TotalFiles
		a.updateProgress(ctx, input.JobID, domain.StageUploading, progress)
		activity.RecordHeartbeat(ctx, progress)
	})
	if err != nil {
		logger.Warn("failed to upload subtitles", zap.Error(err))
	} else {
		allArtifacts = append(allArtifacts, subsArtifacts...)
	}

	// Upload metadata
	metaArtifacts, err := uploader.UploadDirectory(ctx, input.JobID, workspace.Paths().Meta, bucket, prefix+"/meta", nil)
	if err != nil {
		logger.Warn("failed to upload metadata", zap.Error(err))
	} else {
		allArtifacts = append(allArtifacts, metaArtifacts...)
	}

	// Save artifacts to database
	if err := a.artifactRepo.CreateBatch(ctx, allArtifacts); err != nil {
		return nil, fmt.Errorf("failed to save artifacts: %w", err)
	}

	a.updateProgress(ctx, input.JobID, domain.StageUploading, 100)
	logger.Info("artifacts uploaded", zap.Int("count", len(allArtifacts)))

	return &UploadOutput{ArtifactCount: len(allArtifacts)}, nil
}

// CleanupInput holds cleanup input
type CleanupInput struct {
	JobID uuid.UUID `json:"jobId"`
}

// Cleanup cleans up workspace
func (a *Activities) Cleanup(ctx context.Context, input CleanupInput) error {
	logger := a.logger.With(zap.String("jobId", input.JobID.String()), zap.String("activity", "Cleanup"))
	startTime := time.Now()
	defer func() {
		a.metrics.RecordStageDuration(string(domain.StageCleanup), time.Since(startTime).Seconds())
		a.metrics.DecrementJobsActive()
	}()

	if err := a.updateProgress(ctx, input.JobID, domain.StageCleanup, 0); err != nil {
		logger.Error("failed to update progress", zap.Error(err))
	}

	workspace := ffmpeg.NewWorkspace(a.config.Worker.WorkdirRoot, input.JobID)

	if err := workspace.Cleanup(); err != nil {
		logger.Warn("failed to cleanup workspace", zap.Error(err))
	}

	a.updateProgress(ctx, input.JobID, domain.StageCleanup, 100)
	logger.Info("cleanup complete")

	return nil
}

// Helper methods

// startPeriodicHeartbeat starts a goroutine that sends heartbeats every interval
// Returns a cancel function to stop the goroutine
func startPeriodicHeartbeat(ctx context.Context, interval time.Duration, details interface{}) func() {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				activity.RecordHeartbeat(ctx, details)
			}
		}
	}()
	return func() { close(done) }
}

func (a *Activities) updateProgress(ctx context.Context, jobID uuid.UUID, stage domain.Stage, stageProgress int) error {
	job, err := a.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return err
	}

	job.CurrentStage = &stage
	job.StageProgress = stageProgress
	job.OverallProgress = job.CalculateOverallProgress()

	return a.jobRepo.UpdateProgress(ctx, jobID, stage, stageProgress, job.OverallProgress)
}

func (a *Activities) recordError(ctx context.Context, jobID uuid.UUID, stage domain.Stage, code string, err error) error {
	job, _ := a.jobRepo.GetByID(ctx, jobID)
	attempt := 0
	if job != nil {
		attempt = job.Attempt
	}

	class := domain.ClassifyError(code)
	convErr := domain.NewConversionError(jobID, stage, class, code, err.Error(), attempt)
	a.errorRepo.Create(ctx, convErr)

	a.metrics.IncrementStageFailures(string(stage), string(class))

	if class == domain.ErrorClassFatal {
		return temporal.NewNonRetryableApplicationError(err.Error(), code, err)
	}
	return err
}

// FinalizeJobInput holds finalize job input
type FinalizeJobInput struct {
	JobID  uuid.UUID        `json:"jobId"`
	Status domain.JobStatus `json:"status"`
	Error  string           `json:"error,omitempty"`
}

// FinalizeJob updates job status to final state (completed/failed/canceled)
func (a *Activities) FinalizeJob(ctx context.Context, input FinalizeJobInput) error {
	logger := a.logger.With(
		zap.String("jobId", input.JobID.String()),
		zap.String("activity", "FinalizeJob"),
		zap.String("status", string(input.Status)),
	)

	if err := a.jobRepo.SetFinished(ctx, input.JobID, input.Status); err != nil {
		logger.Error("failed to set job finished", zap.Error(err))
		return fmt.Errorf("failed to finalize job: %w", err)
	}

	// Record error if job failed
	if input.Status == domain.JobStatusFailed && input.Error != "" {
		convErr := domain.NewConversionError(
			input.JobID,
			domain.StageUnknown,
			domain.ErrorClassFatal,
			domain.ErrCodeWorkflowFailed,
			input.Error,
			0,
		)
		if err := a.errorRepo.Create(ctx, convErr); err != nil {
			logger.Warn("failed to record error", zap.Error(err))
		}
	}

	// Update metrics
	a.metrics.IncrementJobsTotal(string(input.Status))

	logger.Info("job finalized", zap.String("finalStatus", string(input.Status)))
	return nil
}
