package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration
type Config struct {
	Database   DatabaseConfig
	Temporal   TemporalConfig
	S3         S3Config
	Worker     WorkerConfig
	API        APIConfig
	FFmpeg     FFmpegConfig
	Thumbnails ThumbnailsConfig
	HLS        HLSConfig
	Encoding   EncodingConfig
	DRM        DRMConfig
	Retry      RetryConfig
	Log        LogConfig
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	URL             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// TemporalConfig holds Temporal configuration
type TemporalConfig struct {
	Address   string
	Namespace string
	TaskQueue string
}

// S3Config holds S3 configuration
type S3Config struct {
	Endpoint     string
	Region       string
	AccessKey    string
	SecretKey    string
	BucketOutput string
	UseSSL       bool
}

// WorkerConfig holds worker configuration
type WorkerConfig struct {
	WorkdirRoot       string
	MaxParallelJobs   int
	MaxParallelFFmpeg int
	MaxParallelUploads int
	EnableGPU         bool
}

// APIConfig holds API configuration
type APIConfig struct {
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// FFmpegConfig holds FFmpeg configuration
type FFmpegConfig struct {
	BinaryPath      string
	FFprobePath     string
	ProcessTimeout  time.Duration
}

// ThumbnailsConfig holds thumbnail generation defaults
type ThumbnailsConfig struct {
	MaxFrames int
}

// HLSConfig holds HLS generation defaults
type HLSConfig struct {
	SegmentDurationSec int
	EnableEncryption   bool
	KeyURL             string // URL template for key delivery, e.g., "https://example.com/keys/{job_id}/key"
}

// EncodingConfig holds multi-codec encoding configuration
type EncodingConfig struct {
	// Encoding tiers
	EnableLegacyTier bool // H264/AAC/TS - maximum compatibility
	EnableModernTier bool // H265/AAC/fMP4 - 40% bandwidth savings

	// Container format for HLS segments
	HLSSegmentType string // "ts" or "fmp4"

	// H.265 specific settings
	H265Preset string // CPU preset: ultrafast, superfast, veryfast, faster, fast, medium, slow, slower, veryslow
	H265CRF    int    // Constant Rate Factor (0-51, lower = better quality, 26 recommended)
}

// DRMConfig holds DRM configuration
type DRMConfig struct {
	Enabled            bool
	Provider           string // "widevine", "fairplay", "playready", "all"
	ShakaPackagerPath  string
	KeyServerURL       string // License server URL for key requests
	SignerURL          string // URL for FairPlay certificate
	// Widevine specific
	WidevineKeyID      string
	WidevineKey        string
	WidevinePSSH       string
	// FairPlay specific
	FairPlayKeyURL     string
	FairPlayCertPath   string
	FairPlayIV         string
	// PlayReady specific
	PlayReadyKeyID     string
	PlayReadyKey       string
	PlayReadyLAURL     string // License Acquisition URL
}

// RetryConfig holds retry policy configuration
type RetryConfig struct {
	Count        int
	BaseDelayMs  int
	MaxDelayMs   int
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level  string
	Format string
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		Database: DatabaseConfig{
			URL:             getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/converter?sslmode=disable"),
			MaxOpenConns:    getEnvInt("DATABASE_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvInt("DATABASE_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvDuration("DATABASE_CONN_MAX_LIFETIME", 5*time.Minute),
		},
		Temporal: TemporalConfig{
			Address:   getEnv("TEMPORAL_ADDRESS", "localhost:7233"),
			Namespace: getEnv("TEMPORAL_NAMESPACE", "default"),
			TaskQueue: getEnv("TEMPORAL_TASK_QUEUE", "video-conversion"),
		},
		S3: S3Config{
			Endpoint:     getEnv("S3_ENDPOINT", "http://localhost:9000"),
			Region:       getEnv("S3_REGION", "us-east-1"),
			AccessKey:    getEnv("S3_ACCESS_KEY", ""),
			SecretKey:    getEnv("S3_SECRET_KEY", ""),
			BucketOutput: getEnv("S3_BUCKET_OUTPUT", "converted"),
			UseSSL:       getEnvBool("S3_USE_SSL", false),
		},
		Worker: WorkerConfig{
			WorkdirRoot:        getEnv("WORKDIR_ROOT", "/work"),
			MaxParallelJobs:    getEnvInt("MAX_PARALLEL_JOBS", 2),
			MaxParallelFFmpeg:  getEnvInt("MAX_PARALLEL_FFMPEG", 4),
			MaxParallelUploads: getEnvInt("MAX_PARALLEL_UPLOADS", 10),
			EnableGPU:          getEnvBool("ENABLE_GPU", true),
		},
		API: APIConfig{
			Port:         getEnvInt("API_PORT", 8080),
			ReadTimeout:  getEnvDuration("API_READ_TIMEOUT", 30*time.Second),
			WriteTimeout: getEnvDuration("API_WRITE_TIMEOUT", 30*time.Second),
		},
		FFmpeg: FFmpegConfig{
			BinaryPath:     getEnv("FFMPEG_PATH", "ffmpeg"),
			FFprobePath:    getEnv("FFPROBE_PATH", "ffprobe"),
			ProcessTimeout: getEnvDuration("FFMPEG_PROCESS_TIMEOUT", 6*time.Hour),
		},
		Thumbnails: ThumbnailsConfig{
			MaxFrames: getEnvInt("THUMB_MAX_FRAMES", 200),
		},
		HLS: HLSConfig{
			SegmentDurationSec: getEnvInt("HLS_SEGMENT_DURATION_SEC", 4),
			EnableEncryption:   getEnvBool("HLS_ENABLE_ENCRYPTION", false),
			KeyURL:             getEnv("HLS_KEY_URL", ""),
		},
		Encoding: EncodingConfig{
			EnableLegacyTier: getEnvBool("ENCODING_LEGACY_TIER", true),
			EnableModernTier: getEnvBool("ENCODING_MODERN_TIER", true),
			HLSSegmentType:   getEnv("HLS_SEGMENT_TYPE", "fmp4"),
			H265Preset:       getEnv("H265_PRESET", "medium"),
			H265CRF:          getEnvInt("H265_CRF", 26),
		},
		DRM: DRMConfig{
			Enabled:           getEnvBool("DRM_ENABLED", false),
			Provider:          getEnv("DRM_PROVIDER", "widevine"), // widevine, fairplay, playready, all
			ShakaPackagerPath: getEnv("SHAKA_PACKAGER_PATH", "packager"),
			KeyServerURL:      getEnv("DRM_KEY_SERVER_URL", ""),
			SignerURL:         getEnv("DRM_SIGNER_URL", ""),
			// Widevine
			WidevineKeyID: getEnv("DRM_WIDEVINE_KEY_ID", ""),
			WidevineKey:   getEnv("DRM_WIDEVINE_KEY", ""),
			WidevinePSSH:  getEnv("DRM_WIDEVINE_PSSH", ""),
			// FairPlay
			FairPlayKeyURL:   getEnv("DRM_FAIRPLAY_KEY_URL", ""),
			FairPlayCertPath: getEnv("DRM_FAIRPLAY_CERT_PATH", ""),
			FairPlayIV:       getEnv("DRM_FAIRPLAY_IV", ""),
			// PlayReady
			PlayReadyKeyID: getEnv("DRM_PLAYREADY_KEY_ID", ""),
			PlayReadyKey:   getEnv("DRM_PLAYREADY_KEY", ""),
			PlayReadyLAURL: getEnv("DRM_PLAYREADY_LA_URL", ""),
		},
		Retry: RetryConfig{
			Count:       getEnvInt("RETRY_COUNT", 3),
			BaseDelayMs: getEnvInt("RETRY_BASE_DELAY_MS", 1000),
			MaxDelayMs:  getEnvInt("RETRY_MAX_DELAY_MS", 30000),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "json"),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.S3.AccessKey == "" {
		return fmt.Errorf("S3_ACCESS_KEY is required")
	}
	if c.S3.SecretKey == "" {
		return fmt.Errorf("S3_SECRET_KEY is required")
	}
	if c.S3.BucketOutput == "" {
		return fmt.Errorf("S3_BUCKET_OUTPUT is required")
	}
	if c.Worker.MaxParallelJobs < 1 {
		return fmt.Errorf("MAX_PARALLEL_JOBS must be at least 1")
	}
	if c.Worker.MaxParallelFFmpeg < 1 {
		return fmt.Errorf("MAX_PARALLEL_FFMPEG must be at least 1")
	}
	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}
