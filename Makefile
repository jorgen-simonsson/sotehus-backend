.PHONY: build run test clean dev deps

# Binary name
BINARY=sotehus-backend

# Build the application
build:
	go build -o bin/$(BINARY) ./cmd/server

# Run the application
run: build
	./bin/$(BINARY)

# Run in development mode (with hot reload if air is installed)
dev:
	@if command -v air > /dev/null; then \
		air; \
	else \
		go run ./cmd/server; \
	fi

# Run tests
test:
	go test -v ./...

# Run tests with coverage
test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Download dependencies
deps:
	go mod download
	go mod tidy

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

# Build for multiple platforms
build-all:
	GOOS=linux GOARCH=amd64 go build -o bin/$(BINARY)-linux-amd64 ./cmd/server
	GOOS=linux GOARCH=arm64 go build -o bin/$(BINARY)-linux-arm64 ./cmd/server
	GOOS=darwin GOARCH=amd64 go build -o bin/$(BINARY)-darwin-amd64 ./cmd/server
	GOOS=darwin GOARCH=arm64 go build -o bin/$(BINARY)-darwin-arm64 ./cmd/server

# Build Docker image
docker-build:
	docker build -t $(BINARY):latest .

# Run Docker container
docker-run:
	docker run --rm -p 8080:8080 --env-file .env $(BINARY):latest
