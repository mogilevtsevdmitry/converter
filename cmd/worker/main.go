package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.uber.org/zap"

	"github.com/tvoe/converter/internal/config"
	"github.com/tvoe/converter/internal/db"
	"github.com/tvoe/converter/internal/ffmpeg"
	"github.com/tvoe/converter/internal/metrics"
	"github.com/tvoe/converter/internal/storage/s3"
	"github.com/tvoe/converter/internal/temporal/activities"
	"github.com/tvoe/converter/internal/temporal/workflows"
)

func main() {
	// Load .env file if exists
	_ = godotenv.Load()

	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
	defer logger.Sync()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("failed to load configuration", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize database
	database, err := db.New(ctx, cfg.Database)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer database.Close()

	// Initialize repositories
	jobRepo := db.NewJobRepository(database)
	errorRepo := db.NewErrorRepository(database)
	artifactRepo := db.NewArtifactRepository(database)

	// Initialize S3 client
	s3Client, err := s3.New(cfg.S3)
	if err != nil {
		logger.Fatal("failed to initialize S3 client", zap.Error(err))
	}

	// Initialize Temporal client
	temporalClient, err := client.Dial(client.Options{
		HostPort:  cfg.Temporal.Address,
		Namespace: cfg.Temporal.Namespace,
	})
	if err != nil {
		logger.Fatal("failed to connect to Temporal", zap.Error(err))
	}
	defer temporalClient.Close()

	// Initialize metrics
	m := metrics.New()

	// Create activities
	acts := activities.NewActivities(
		cfg,
		jobRepo,
		errorRepo,
		artifactRepo,
		s3Client,
		logger,
		m,
	)

	// Create worker
	w := worker.New(temporalClient, cfg.Temporal.TaskQueue, worker.Options{
		MaxConcurrentActivityExecutionSize:     cfg.Worker.MaxParallelJobs,
		MaxConcurrentWorkflowTaskExecutionSize: cfg.Worker.MaxParallelJobs * 2,
	})

	// Register workflows
	w.RegisterWorkflow(workflows.VideoConversionWorkflow)

	// Register activities
	w.RegisterActivity(acts.ExtractMetadata)
	w.RegisterActivity(acts.ValidateInputs)
	w.RegisterActivity(acts.Transcode)
	w.RegisterActivity(acts.ExtractSubtitles)
	w.RegisterActivity(acts.GenerateThumbnails)
	w.RegisterActivity(acts.SegmentHLS)
	w.RegisterActivity(acts.UploadArtifacts)
	w.RegisterActivity(acts.Cleanup)

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start metrics server
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		})
		metricsAddr := ":9090"
		logger.Info("starting metrics server", zap.String("addr", metricsAddr))
		if err := http.ListenAndServe(metricsAddr, mux); err != nil {
			logger.Error("metrics server error", zap.Error(err))
		}
	}()

	// Start disk space monitoring
	go monitorDiskSpace(ctx, cfg.Worker.WorkdirRoot, m, logger)

	// Start orphan cleanup
	go runOrphanCleanup(ctx, cfg.Worker.WorkdirRoot, logger)

	// Start worker in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- w.Run(worker.InterruptCh())
	}()

	logger.Info("worker started",
		zap.String("taskQueue", cfg.Temporal.TaskQueue),
		zap.Int("maxParallelJobs", cfg.Worker.MaxParallelJobs),
		zap.Bool("gpuEnabled", cfg.Worker.EnableGPU),
	)

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		logger.Info("received shutdown signal", zap.String("signal", sig.String()))
	case err := <-errChan:
		if err != nil {
			logger.Error("worker error", zap.Error(err))
		}
	}

	cancel()
	w.Stop()
	logger.Info("worker stopped")
}

// monitorDiskSpace monitors disk space and updates metrics
func monitorDiskSpace(ctx context.Context, workdir string, m *metrics.Metrics, logger *zap.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var stat syscall.Statfs_t
			if err := syscall.Statfs(workdir, &stat); err != nil {
				logger.Warn("failed to get disk stats", zap.Error(err))
				continue
			}

			freeBytes := float64(stat.Bavail) * float64(stat.Bsize)
			m.SetDiskFreeBytes(freeBytes)

			// Log warning if disk space is low (less than 10GB)
			if freeBytes < 10*1024*1024*1024 {
				logger.Warn("low disk space",
					zap.Float64("freeGB", freeBytes/1024/1024/1024),
				)
			}
		}
	}
}

// runOrphanCleanup periodically cleans up orphan workspaces
func runOrphanCleanup(ctx context.Context, workdir string, logger *zap.Logger) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			logger.Info("running orphan workspace cleanup")
			if err := ffmpeg.CleanupOrphans(workdir, 24*time.Hour); err != nil {
				logger.Warn("orphan cleanup failed", zap.Error(err))
			}
		}
	}
}
