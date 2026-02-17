BINARY_NAME=faucet
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

.PHONY: all build build-ui build-go test lint dev clean bench install

all: build

## Build
build: build-ui build-go

build-go:
	CGO_ENABLED=0 go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/faucet

build-ui:
	@if [ -d "ui/node_modules" ]; then \
		cd ui && npm run build; \
	else \
		echo "UI not built (run 'cd ui && npm install' first)"; \
		mkdir -p internal/ui/dist && touch internal/ui/dist/.gitkeep; \
	fi

## Development
dev:
	go run $(LDFLAGS) ./cmd/faucet serve --dev

dev-ui:
	cd ui && npm run dev

## Testing
test:
	go test -race -count=1 ./...

test-v:
	go test -race -count=1 -v ./...

test-cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

## Integration tests (require Docker)
test-integration:
	go test -race -tags=integration -count=1 -v ./...

## Linting
lint:
	golangci-lint run ./...

## Benchmarks
bench:
	go test -bench=. -benchmem ./...

## Install locally
install: build
	cp bin/$(BINARY_NAME) $(GOPATH)/bin/$(BINARY_NAME)

## Clean
clean:
	rm -rf bin/ coverage.out coverage.html
	rm -rf internal/ui/dist

## Dependencies
deps:
	go mod tidy
	go mod verify

## Format
fmt:
	gofmt -s -w .
	goimports -w .

## Generate (if needed)
generate:
	go generate ./...

## Docker
docker:
	docker build -t $(BINARY_NAME):$(VERSION) .

## Release (requires goreleaser)
release:
	goreleaser release --clean

## Help
help:
	@echo "Available targets:"
	@echo "  build       - Build binary (UI + Go)"
	@echo "  build-go    - Build Go binary only"
	@echo "  build-ui    - Build frontend UI"
	@echo "  dev         - Run in development mode"
	@echo "  test        - Run tests"
	@echo "  test-cover  - Run tests with coverage"
	@echo "  lint        - Run linter"
	@echo "  bench       - Run benchmarks"
	@echo "  clean       - Remove build artifacts"
	@echo "  deps        - Tidy and verify dependencies"
	@echo "  docker      - Build Docker image"
