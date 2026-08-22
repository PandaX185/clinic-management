# Makefile for Axiom

.PHONY: help build run test test-race test-coverage lint fmt migrate-up migrate-down migrate-create sqlc-generate docker-up docker-down docker-build sqlc-verify

# Default target
help:
	@echo "Axiom - Available Commands:"
	@echo ""
	@echo "Build & Run:"
	@echo "  build           - Build the application"
	@echo "  run             - Run the application locally"
	@echo "  docker-up       - Start dependencies (PostgreSQL, Redis, NATS)"
	@echo "  docker-down     - Stop dependencies"
	@echo "  docker-build    - Build Docker image"
	@echo ""
	@echo "Database:"
	@echo "  migrate-up      - Run all migrations up"
	@echo "  migrate-down    - Rollback last migration"
	@echo "  migrate-create  - Create new migration (usage: make migrate-create NAME=name)"
	@echo "  sqlc-generate   - Generate Go code from SQL queries"
	@echo "  sqlc-verify     - Verify sqlc configuration"
	@echo ""
	@echo "Testing & Quality:"
	@echo "  test            - Run tests"
	@echo "  test-race       - Run tests with race detector"
	@echo "  test-coverage   - Run tests with coverage report"
	@echo "  lint            - Run linters (golangci-lint)"
	@echo "  fmt             - Format code (gofmt, goimports)"
	@echo ""

# Build
build:
	@echo "Building application..."
	@go build -o bin/api ./cmd/api

run: build
	@echo "Running application..."
	@./bin/api

# Docker
docker-up:
	@echo "Starting dependencies..."
	@docker-compose up -d

docker-down:
	@echo "Stopping dependencies..."
	@docker-compose down

docker-build:
	@echo "Building Docker image..."
	@docker build -t axiom:latest .

# Database Migrations
migrate-up:
	@echo "Running migrations up..."
	@migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down:
	@echo "Rolling back last migration..."
	@migrate -path migrations -database "$(DATABASE_URL)" down 1

migrate-create:
	@if [ -z "$(NAME)" ]; then echo "Usage: make migrate-create NAME=migration_name"; exit 1; fi
	@echo "Creating migration: $(NAME)"
	@migrate create -ext sql -dir migrations -seq $(NAME)

# sqlc
sqlc-generate:
	@echo "Generating sqlc code..."
	@sqlc generate

sqlc-verify:
	@echo "Verifying sqlc configuration..."
	@sqlc vet

# Testing
test:
	@echo "Running tests..."
	@go test ./...

test-race:
	@echo "Running tests with race detector..."
	@go test -race ./...

test-coverage:
	@echo "Running tests with coverage..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Linting & Formatting
lint:
	@echo "Running linters..."
	@golangci-lint run ./...

fmt:
	@echo "Formatting code..."
	@gofmt -w .
	@goimports -w .

# Development
dev: docker-up migrate-up run

# Clean
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@rm -f coverage.out coverage.html