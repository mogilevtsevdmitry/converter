.PHONY: build run test clean docker-build docker-up docker-down migrate

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOCLEAN=$(GOCMD) clean
GOMOD=$(GOCMD) mod

# Binary names
API_BINARY=converter-api
WORKER_BINARY=converter-worker

# Build targets
build: build-api build-worker

build-api:
	$(GOBUILD) -o bin/$(API_BINARY) ./cmd/api

build-worker:
	$(GOBUILD) -o bin/$(WORKER_BINARY) ./cmd/worker

# Run targets
run-api:
	$(GOCMD) run ./cmd/api

run-worker:
	$(GOCMD) run ./cmd/worker

# Test targets
test:
	$(GOTEST) -v ./...

test-coverage:
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Clean
clean:
	$(GOCLEAN)
	rm -rf bin/
	rm -f coverage.out coverage.html

# Dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Docker targets
docker-build:
	docker build -t converter-api:latest -f deploy/docker/Dockerfile.api .
	docker build -t converter-worker:latest -f deploy/docker/Dockerfile.worker .

docker-build-gpu:
	docker build -t converter-worker-gpu:latest -f deploy/docker/Dockerfile.worker.gpu .

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f

docker-logs-api:
	docker-compose logs -f api

docker-logs-worker:
	docker-compose logs -f worker

# Database migrations
migrate-up:
	migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path migrations -database "$(DATABASE_URL)" down

migrate-create:
	migrate create -ext sql -dir migrations -seq $(name)

# Development helpers
dev-deps:
	docker-compose up -d postgres temporal minio minio-init

dev-stop:
	docker-compose stop postgres temporal temporal-ui minio

lint:
	golangci-lint run ./...

fmt:
	gofmt -s -w .

# API testing
test-create-job:
	curl -X POST http://localhost:8080/v1/jobs \
		-H "Content-Type: application/json" \
		-d '{"source":{"type":"s3","bucket":"source","key":"test.mp4"},"profile":{"qualities":["480p","720p","1080p"]}}'

test-get-job:
	curl http://localhost:8080/v1/jobs/$(JOB_ID)

test-health:
	curl http://localhost:8080/healthz

help:
	@echo "Available targets:"
	@echo "  build          - Build all binaries"
	@echo "  build-api      - Build API binary"
	@echo "  build-worker   - Build Worker binary"
	@echo "  run-api        - Run API locally"
	@echo "  run-worker     - Run Worker locally"
	@echo "  test           - Run tests"
	@echo "  test-coverage  - Run tests with coverage"
	@echo "  clean          - Clean build artifacts"
	@echo "  deps           - Download dependencies"
	@echo "  docker-build   - Build Docker images"
	@echo "  docker-up      - Start all services"
	@echo "  docker-down    - Stop all services"
	@echo "  docker-logs    - View logs"
	@echo "  dev-deps       - Start development dependencies"
	@echo "  lint           - Run linter"
	@echo "  fmt            - Format code"
