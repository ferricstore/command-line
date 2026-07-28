GO ?= go
GOLANGCI_LINT ?= golangci-lint
GORELEASER ?= goreleaser

.PHONY: build test test-race integration-login lint fmt tidy verify snapshot clean

build:
	$(GO) build -o bin/ferric ./cmd/ferric

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

integration-login:
	./scripts/integration-login-oss.sh

lint:
	$(GOLANGCI_LINT) run ./...

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy

verify:
	$(GO) mod verify
	$(GO) vet ./...
	$(GO) test ./...

snapshot:
	$(GORELEASER) release --snapshot --clean

clean:
	$(GO) clean
	rm -rf bin dist coverage
