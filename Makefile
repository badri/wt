.PHONY: build test test-unit test-integration coverage lint clean

# Build the binary
build:
	go build -o wt ./cmd/wt

# Run all tests (unit only by default)
test: test-unit

# Run unit tests
test-unit:
	go test -v -race ./...

# Run integration tests (requires tmux)
test-integration:
	go test -tags=integration -v ./test/integration/...

# Run tests with coverage
coverage:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Show coverage summary
coverage-summary:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

# Run linter
lint:
	go vet ./...
	@if [ "$$(gofmt -s -l . | wc -l)" -gt 0 ]; then \
		echo "Files need formatting:"; \
		gofmt -s -l .; \
		exit 1; \
	fi

# Format code
fmt:
	gofmt -s -w .

# Clean build artifacts
clean:
	rm -f wt coverage.out coverage.html

# Install binary to GOPATH/bin
install:
	go install ./cmd/wt

# Run all checks (what CI does)
ci: lint test-unit build
