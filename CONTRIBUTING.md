# Contributing

Thank you for helping build the Ferric command-line interface.

## Requirements

- Go 1.24 or newer
- mise for the pinned development toolchain
- golangci-lint 2.12.2 for local lint runs
- GoReleaser 2.17.0 for local release snapshots

## Setup

~~~sh
brew install mise
mise trust ./mise.toml
mise exec -- go mod download
~~~

## Before Opening a Pull Request

Run the same core checks used by CI:

~~~sh
mise exec -- gofmt -w .
mise exec -- go mod tidy
mise exec -- go vet ./...
mise exec -- go test ./...
mise exec -- go test -race ./...
golangci-lint run ./...
./scripts/integration-login-oss.sh
~~~

Keep changes focused and add tests for user-visible behavior. Construct Cobra
commands without package-level mutable state so tests and embedded use remain
isolated.

## Adding Commands

- Put command wiring in internal/cli.
- Keep FerricStore SDK setup behind internal/ferric.
- Accept dependencies through constructors when a command needs network access.
- Never print credentials, authentication URLs, or raw secret-bearing config.
- Return errors to the root command instead of exiting from subcommands.

## Authentication Testing Boundary

This repository runs real username/password login tests against the released
OSS FerricStore container. Enterprise SSO and API-token provider behavior must
use mocks here. End-to-end Enterprise authentication belongs in the private
Enterprise repository and should execute the public CLI as a black box.

## Pull Requests

Describe the user impact, include relevant test coverage, and note any release
or compatibility concerns. Pull requests must pass formatting, lint, unit,
race, vulnerability, CodeQL, and release-snapshot checks.
