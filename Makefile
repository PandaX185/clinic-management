.PHONY: help build run test test-race test-coverage lint vet fmt sqlc swagger migrate-up migrate-down docker-build docker-up docker-down tidy

space := $(eval) $(eval)
comma := ,

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

# The repo root is not a Go package, which swag chokes on when using -d . as a
# search dir (every parsed file gets dropped). Search each real API package and
# keep cmd/api first since -g main.go is resolved relative to the first dir.
SWAGGER_DIRS := cmd/api $(shell find internal -type d -name api -not -path '*/docs/*' -not -path '*/vendor/*' | sort)

swagger:
	go run github.com/swaggo/swag/cmd/swag@v1.16.4 init -g main.go -d "$(subst $(space),$(comma),$(SWAGGER_DIRS))" -o docs --parseInternal=true --parseDependency=true

migrate-up:
	migrate -path ./db/migrations/global -database "$${DATABASE_URL:-postgres://lahza:lahza@localhost:5432/lahza?sslmode=disable}" up

migrate-down:
	migrate -path ./db/migrations/global -database "$${DATABASE_URL:-postgres://lahza:lahza@localhost:5432/lahza?sslmode=disable}" down 1

docker-build:
	docker build -t lahza-api .

docker-up:
	docker compose up -d

docker-down:
	docker compose down

tidy:
	go mod tidy
