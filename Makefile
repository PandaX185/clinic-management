.PHONY: help build run test test-race test-coverage lint vet fmt sqlc swagger migrate-up migrate-down docker-build docker-up docker-down tidy

help:
	@echo "Targets:"
	@echo "  build          - compile the api binary into bin/"
	@echo "  run            - start the api server locally"
	@echo "  test           - run unit tests"
	@echo "  test-race      - run tests with race detector"
	@echo "  test-coverage  - coverage profile + HTML report"
	@echo "  lint           - golangci-lint (if installed)"
	@echo "  vet            - go vet ./..."
	@echo "  fmt            - gofmt all sources"
	@echo "  sqlc           - regenerate db layer"
	@echo "  swagger        - regenerate swagger docs (swag init)"
	@echo "  migrate-up     - apply all migrations"
	@echo "  migrate-down   - revert one migration"
	@echo "  docker-build   - build production image"
	@echo "  docker-up      - start full stack via compose"
	@echo "  docker-down    - stop compose stack"

build:
	go build -o bin/api ./cmd/api

run: 
	go run ./cmd/api

test:
	go test ./... 

test-race:
	go test -race ./...

test-coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || echo "golangci-lint not installed"

vet:
	go vet ./...

fmt:
	gofmt -s -w .

sqlc:
	sqlc generate

swagger:
	swag init -g cmd/api/main.go -d . -o docs --parseInternal=true

migrate-up:
	migrate -path ./db/migrations/global -database "$${DATABASE_URL:-postgres://clinic:clinic@localhost:5432/clinic?sslmode=disable}" up

migrate-down:
	migrate -path ./db/migrations/global -database "$${DATABASE_URL:-postgres://clinic:clinic@localhost:5432/clinic?sslmode=disable}" down 1

docker-build:
	docker build -t clinic-management-api .

docker-up:
	docker compose up -d

docker-down:
	docker compose down

tidy:
	go mod tidy
