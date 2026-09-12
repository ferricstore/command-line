GO ?= go
GOLANGCI_LINT ?= golangci-lint
GORELEASER ?= goreleaser
CONTAINER_IMAGE ?= ferric:dev

.PHONY: build container test test-container test-race integration-login integration-oss integration-http lint fmt tidy verify snapshot clean

build:
	$(GO) build -o bin/ferric ./cmd/ferric

container:
	docker build --build-arg VERSION=dev --tag $(CONTAINER_IMAGE) .

test:
	$(GO) test ./...
	./scripts/verify-release-version_test.sh
	./scripts/resolve-ferricstore-image_test.sh

test-container:
	./scripts/test-container.sh

test-race:
	$(GO) test -race ./...

integration-oss:
	./scripts/integration-login-oss.sh

integration-login: integration-oss

integration-http:
	./scripts/integration-http-tls.sh

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
	./scripts/verify-release-version_test.sh
	./scripts/resolve-ferricstore-image_test.sh

snapshot:
	$(GORELEASER) release --snapshot --clean

clean:
	$(GO) clean
	rm -rf bin dist coverage
