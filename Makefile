.PHONY: help install fmt lint staticcheck vuln vet build test race check release

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

check: fmt vet lint staticcheck vuln build test ## Run the local verification suite

# Local release. CI is workflow_dispatch-only until signing creds are in
# CI; until then every release runs locally. Requires 1Password signed
# in, the v$(VERSION) tag at HEAD, and a keychain profile named
# "notarytool" (xcrun notarytool store-credentials).
release: ## Local signed + notarized release (usage: make release VERSION=X.Y.Z)
	@test -n "$(VERSION)" || (echo "usage: make release VERSION=X.Y.Z" && exit 2)
	@op whoami >/dev/null || (echo "1Password not signed in: eval \$$(op signin)" && exit 1)
	@gh auth status >/dev/null 2>&1 || (echo "gh not authenticated: gh auth login" && exit 1)
	@xcrun notarytool history --keychain-profile notarytool >/dev/null 2>&1 || \
	  (echo "keychain profile 'notarytool' missing — see scripts/notarize-darwin.sh header"; exit 1)
	@existing=$$(git rev-parse -q --verify "v$(VERSION)^{commit}" 2>/dev/null); \
	head=$$(git rev-parse HEAD); \
	test -n "$$existing" && test "$$existing" = "$$head" || \
	  (echo "v$(VERSION) must exist and point at HEAD before release"; exit 3)
	# Build + codesign + archive + publish + commit Cask back to the tap.
	GITHUB_TOKEN="$$(gh auth token)" \
	  jwa-harden run -- goreleaser release --clean
	# Submit each codesigned darwin binary to notarytool.
	scripts/notarize-darwin.sh prehandover $(VERSION)
