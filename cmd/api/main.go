package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"

	"github.com/tvoe/converter/internal/api"
	"github.com/tvoe/converter/internal/config"
	"github.com/tvoe/converter/internal/db"
	"github.com/tvoe/converter/internal/metrics"
	"github.com/tvoe/converter/internal/storage/s3"
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

	// Initialize handler
	handler := api.NewHandler(
		cfg,
		jobRepo,
		errorRepo,
		artifactRepo,
		s3Client,
		temporalClient,
		logger,
		m,
	)

	// Create router
	router := api.NewRouter(handler, logger)

	// Create server
	server := api.NewServer(cfg.API, router, logger)

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := server.Start(); err != nil {
			logger.Error("server error", zap.Error(err))
			cancel()
		}
	}()

	logger.Info("API server started",
		zap.Int("port", cfg.API.Port),
		zap.String("temporalAddress", cfg.Temporal.Address),
	)

	// Wait for shutdown signal
	<-sigChan
	logger.Info("received shutdown signal")

	if err := server.Stop(ctx); err != nil {
		logger.Error("error during shutdown", zap.Error(err))
	}

	logger.Info("API server stopped")
}
