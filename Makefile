.PHONY: help install fmt lint staticcheck vuln vet build test race check

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

install: ## Install the current CLI into GOPATH/bin
	go install ./cmd/prehandover

fmt: ## Check Go formatting
	./scripts/gofmt-strict.sh

lint: ## Run golangci-lint
	golangci-lint run ./...

staticcheck: ## Run repo-pinned Staticcheck
	go tool staticcheck ./...

vuln: ## Run repo-pinned govulncheck
	go tool govulncheck ./...

vet: ## Run go vet
	go vet ./...

build: ## Build all packages
	go build ./...

test: ## Run tests
	go test ./...

race: ## Run tests with race detector
	go test -race ./...

check: fmt vet staticcheck vuln build test ## Run the local verification suite
