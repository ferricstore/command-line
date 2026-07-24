# Ferric Command Line

[![CI](https://github.com/ferricstore/command-line/actions/workflows/ci.yml/badge.svg)](https://github.com/ferricstore/command-line/actions/workflows/ci.yml)
[![Security](https://github.com/ferricstore/command-line/actions/workflows/security.yml/badge.svg)](https://github.com/ferricstore/command-line/actions/workflows/security.yml)

The official command-line interface for FerricStore and FerricFlow.

This repository is in its infrastructure-first stage. It provides the command
framework, FerricStore Go SDK integration boundary, tests, linting, security
checks, and cross-platform release automation. Product commands will be added
incrementally.

## Install

Until the first release, install from the main branch:

~~~sh
go install github.com/ferricstore/command-line/cmd/ferric@main
~~~

After releases begin, install the latest tagged version:

~~~sh
go install github.com/ferricstore/command-line/cmd/ferric@latest
~~~

Release archives for Linux, macOS, and Windows will also be available from the
[GitHub Releases page](https://github.com/ferricstore/command-line/releases).

## Usage

~~~text
ferric
ferric version
ferric completion bash
ferric completion zsh
ferric completion fish
ferric completion powershell
~~~

The executable is named ferric. Commands that connect to FerricStore will use
the official [FerricStore Go SDK](https://github.com/ferricstore/ferricstore-go).

## Development

Go 1.24 or newer is supported. The repository pins the current development
toolchain through mise:

~~~sh
brew install mise
mise trust ./mise.toml
mise exec -- go mod download
mise exec -- make verify
~~~

Useful targets:

~~~sh
mise exec -- make build
mise exec -- make test
mise exec -- make test-race
mise exec -- make lint
mise exec -- make snapshot
~~~

## Releases

Pushing a semantic-version tag such as v0.1.0 runs the complete test suite and
publishes compressed binaries plus checksums through GoReleaser. See
[RELEASE.md](RELEASE.md) for the release checklist.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Security issues should follow
[SECURITY.md](SECURITY.md).

## License

Apache License 2.0. See [LICENSE](LICENSE).
