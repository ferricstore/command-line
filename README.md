# Ferric Command Line

[![CI](https://github.com/ferricstore/command-line/actions/workflows/ci.yml/badge.svg)](https://github.com/ferricstore/command-line/actions/workflows/ci.yml)
[![Security](https://github.com/ferricstore/command-line/actions/workflows/security.yml/badge.svg)](https://github.com/ferricstore/command-line/actions/workflows/security.yml)

The official command-line interface for FerricStore and FerricFlow.

The CLI is non-interactive and designed for terminals, scripts, and CI. It uses
the official Go SDK for FerricStore data, FerricFlow queues and workflows,
schedules, cluster operations, ACLs, and governance controls. An interactive
Redis/SQL-style shell is intentionally not part of the current scope.

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
ferric auth login --url ferric://127.0.0.1:6388 --username default
ferric auth status
ferric auth logout
ferric server ping
ferric store set user:42 active EX 300
ferric store get user:42
ferric queue enqueue email email-42 '{"to":"ada@example.com"}' --json
ferric queue claim email --worker mailer-1
ferric workflow describe email-42
ferric workflow history email-42
ferric workflow schedule list
ferric cluster health
ferric version
ferric completion bash
ferric completion zsh
ferric completion fish
ferric completion powershell
~~~

The executable is named `ferric`. Commands that connect to FerricStore use the
official [FerricStore Go SDK](https://github.com/ferricstore/ferricstore-go).
See the complete [command design and vocabulary](docs/commands.md).

## Command model

Top-level service groups keep a large product surface predictable:

| Service | Purpose |
| --- | --- |
| `store` | Redis-shaped strings, hashes, lists, sets, sorted sets, streams, specialized structures, and Ferric-native data operations |
| `queue` | Enqueue/send, claim/receive, complete/ack, retry/nack, fail, cancel, history, statistics, and policy |
| `workflow` | Executions, policies, one-shot/recurring schedules, and governance controls |
| `server` / `cluster` | Health, metadata, diagnostics, topology, slots, and membership operations |
| `acl` | OSS ACL user and rule administration |
| `namespace` / `quota` | Resource and quota usage |
| `pubsub` | One-shot publish and channel/subscriber inspection |

Every command has `--help` and examples. Shell completion recommends services,
operations, enum values, and saved profile names without reading credentials.

~~~sh
ferric queue claim --help
ferric workflow signal --help
ferric workflow schedule create --help
~~~

Network commands share global output and timeout controls:

~~~sh
ferric --output json --timeout 30s workflow describe order-42
ferric --output raw store mget user:1 user:2
~~~

`--output auto` prints scalars directly and structured results as readable
JSON. `json` always emits JSON and `raw` emits pipeline-friendly lines. For
Redis-shaped store commands, put global flags before the first protocol
argument so negative indexes and Redis option tokens remain untouched.

## OSS Login

Validate an OSS FerricStore username and password:

~~~sh
ferric auth login \
  --url ferric://store.example.com:6388 \
  --username operator
~~~

The interactive prompt hides the password. For automation, read it from
standard input:

~~~sh
printf '%s\n' "$FERRIC_PASSWORD" | ferric auth login \
  --url ferric://store.example.com:6388 \
  --username operator \
  --password-stdin
~~~

Successful login stores non-secret profile metadata in the user configuration
directory and stores the password in the operating-system keyring. Use
--no-store to validate credentials without persisting either.

Verify that the saved credential can authenticate a real connection:

~~~sh
ferric auth status
~~~

Log out locally without deleting the saved connection metadata:

~~~sh
ferric auth logout
~~~

Logout does not change or revoke an OSS ACL password on the server. Use
`ferric profile delete <name>` when both the connection metadata and its local
credential should be removed.

Both ferric:// and ferrics:// support username/password login. Transport
selection is the user's or administrator's responsibility; ferrics:// adds
TLS.

After login, normal commands load the selected profile and password, open an
authenticated SDK client, execute the operation, and close it. Login does not
leave a background TCP process running. The SDK reapplies authentication if it
has to reconnect while a command is running.

Check a saved connection with an optional PING message:

~~~sh
ferric server ping
ferric server ping hello
~~~

The CLI automatically uses the default connection. Most users do not need to
select or manage named profiles.

### Multiple Connections

Users who manage several environments can create and select named connections:

~~~sh
ferric --profile production auth login \
  --url ferrics://store.example.com:6388 \
  --username operator

ferric profile list
ferric profile show production
ferric profile use production
ferric profile delete production
~~~

After `profile use`, normal commands use that connection without another flag.
An explicit `--profile` or the `FERRIC_PROFILE` environment variable provides a
temporary override. Shell completion suggests saved names for profile commands
and profile overrides. Profile output contains metadata only and never reads or
prints the stored password or token.

Normal help intentionally hides `--profile`: the selected/default connection
is the common path. Advanced users can still use `ferric --profile <name> ...`
or `FERRIC_PROFILE` for a one-command override.

Enterprise SSO and service-account API tokens use the same provider boundary,
but their live control-plane integration is tested and owned by the Enterprise
repository. See [docs/authentication.md](docs/authentication.md).

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
mise exec -- make integration-login
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
