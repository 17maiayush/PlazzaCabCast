.DEFAULT_GOAL := help
PKGS := ./...

help: ## list targets
	@grep -E '^[a-zA-Z0-9_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "} {printf "  %-8s %s\n", $$1, $$2}'

run: ## start the API and console; reads .env if present (see configs/.env.example)
	@if [ -f .env ]; then set -a; . ./.env; set +a; fi; go run ./cmd/server

build: ## compile to bin/server
	go build -o bin/server ./cmd/server

test: ## unit tests under the race detector
	go test -race $(PKGS)

cover: ## tests plus the 100% coverage gate
	go test -race -covermode=atomic -coverpkg=$(PKGS) -coverprofile=cover.out $(PKGS)
	@go tool cover -func=cover.out | tail -n 1
	@go tool cover -func=cover.out | tail -n 1 | grep -q '100.0%' || { echo "coverage gate failed: below 100%"; exit 1; }

lint: ## gofmt, go vet, golangci-lint when installed
	@unformatted=$$(gofmt -l .); if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi
	go vet $(PKGS)
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run $(PKGS); else echo "golangci-lint not installed, skipped"; fi

fuzz: ## explore fare invariants with random inputs for FUZZTIME (default 20s)
	go test -run=^$$ -fuzz=FuzzCalculate -fuzztime=$(or $(FUZZTIME),20s) ./internal/pricing

check: lint cover ## everything CI would run


demo: ## curl walkthrough against a running server
	./scripts/demo.sh

e2e: ## headless-Chrome walkthrough of the console; needs a freshly started server
	cd test/e2e && npm install --silent && node console.mjs

.PHONY: help run build test cover lint check fuzz demo e2e
