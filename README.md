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

CLI release tags match the FerricStore OSS release they are based on. For
example, CLI `v0.11.4` uses SDK `v0.11.4` and targets FerricStore OSS `v0.11.4`
or newer:

~~~sh
go install github.com/ferricstore/command-line/cmd/ferric@v0.11.4
~~~

Release archives for Linux, macOS, and Windows will also be available from the
[GitHub Releases page](https://github.com/ferricstore/command-line/releases).

After the first release is published and the GHCR package is made public,
Docker, Kubernetes utility pods, and CI can run the official one-shot image:

~~~sh
docker run --rm ghcr.io/ferricstore/command-line:v0.11.4 version
kubectl run ferric --rm -it --restart=Never \
  --image=ghcr.io/ferricstore/command-line:v0.11.4 -- version
~~~

The same release publishes one image index for Linux amd64 and arm64. See the
[container guide](docs/containers.md) for secret-file authentication, hardened
execution, Kubernetes, and copying the binary into another image.

The initial macOS archives are not Developer ID signed or notarized. Until
signing is configured, `go install` is the recommended macOS installation
path for environments that require verified local build provenance.

## Usage

~~~text
ferric
ferric auth login --url ferric://127.0.0.1:6388 --username default
ferric auth login --method enterprise-api-token --control-url https://platform.example.com --organization acme --cluster CLUSTER_ID --token-stdin
ferric auth status
ferric auth logout
ferric server ping
ferric store set user:42 active EX 300
ferric store get user:42
ferric queue enqueue email email-42 '{"to":"ada@example.com"}' --json
ferric queue claim email --worker mailer-1
ferric workflow describe email-42
ferric workflow history email-42
ferric workflow query 'FROM runs WHERE partition_key = @partition AND type = @type LIMIT 20 RETURN RECORDS' --param partition=tenant-a --param type=order
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

Batch queue and workflow lifecycle operations use strict, bounded snake_case
JSON files (or stdin) so large operations remain reviewable and scriptable.
See the [batch command contract](docs/commands.md#workflow).

## Command model

Top-level service groups keep a large product surface predictable:

| Service | Purpose |
| --- | --- |
| `store` | Redis-shaped strings, hashes, lists, sets, sorted sets, streams, specialized structures, and Ferric-native data operations |
| `queue` | Enqueue/send, claim/receive, complete/ack, retry/nack, fail, cancel, history, statistics, and policy |
| `workflow` | Executions, bounded FQL queries and plans, query-index status, policies, one-shot/recurring schedules, and governance controls |
| `server` / `cluster` | Health, metadata, diagnostics, topology, slots, and membership operations |
| `acl` | OSS ACL user and rule administration |
| `namespace` / `quota` | Resource and quota usage |
| `pubsub` | One-shot publish and channel/subscriber inspection |

Every command has `--help` and examples. Shell completion recommends services,
operations, enum values, and saved profile names without reading credentials.

~~~sh
ferric queue claim --help
ferric workflow signal --help
ferric workflow query --help
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
argument so negative indexes and Redis option tokens remain untouched. Typed
SDK responses use explicit CLI-owned output schemas; SDK transport snapshots
and newly added SDK fields never become public CLI output implicitly.

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

## Platform Login

Human users receive a time-limited `fsp_user_` token from an authenticated
Platform session. Services use an `fsp_sa_` service-account token whose
allowlist includes `client_session.issue` and whose service account has the same
current RBAC grant for the target cluster. Validate and store either token from
standard input:

~~~sh
printf '%s\n' "$FERRIC_PLATFORM_TOKEN" | ferric auth login \
  --method enterprise-sso \
  --control-url https://platform.example.com \
  --organization acme \
  --cluster 018f20dc-7c39-7f16-9fa8-7e807f9a0f48 \
  --token-stdin
~~~

Use `--method enterprise-api-token` for a service-account token. Login performs
a real Platform exchange and native PING before saving anything. Normal commands
exchange again for a 15-minute, namespace-scoped Enterprise credential and send
only that temporary username/password to FerricStore. The control-plane token
stays in the operating-system keyring and is never forwarded to the data plane.
Platform URLs require HTTPS except for loopback development.

Check a saved connection with an optional PING message:

~~~sh
ferric server ping
ferric server ping hello
~~~

### Environment Credentials

For CI, containers, and other ephemeral processes, commands can authenticate
directly from an AWS-style environment credential set without running
`ferric auth login`, writing a profile, or accessing the operating-system
keyring:

~~~sh
FERRIC_URL=ferrics://store.example.com:6388 \
FERRIC_USERNAME=operator \
FERRIC_PASSWORD="$PASSWORD" \
ferric store get user:42
~~~

Mounted secret files are preferred in Docker and Kubernetes:

~~~sh
FERRIC_URL=ferrics://store.example.com:6388 \
FERRIC_USERNAME=operator \
FERRIC_PASSWORD_FILE=/run/secrets/ferric-password \
ferric server ping
~~~

Set exactly one of `FERRIC_PASSWORD` or `FERRIC_PASSWORD_FILE`. The URL,
username, and secret form one atomic credential source; the CLI never fills a
missing environment value from a saved profile or keyring.

Enterprise builds use `FERRIC_CONTROL_URL`, `FERRIC_ORGANIZATION`,
`FERRIC_CLUSTER`, and exactly one of `FERRIC_API_TOKEN` or
`FERRIC_API_TOKEN_FILE`. Commands exchange the service token through Platform
and authenticate to the packaged Enterprise data plane with the returned
short-lived native credential.

An explicit `--profile` wins over environment credentials. Otherwise, a
complete direct environment set wins over `FERRIC_PROFILE`, the selected
profile, and the default profile. `ferric auth status` reports the active
source. Environment credentials are process-owned, so `ferric auth logout`
asks the user to unset them and never modifies a saved credential.

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

## Compatibility

This revision uses FerricStore Go SDK v0.11.4 and targets FerricStore 0.11.4 or
newer. FQL query, query-index, and the current typed schedule response contracts
require that server generation.

## Development

Go 1.25.12 or newer is supported. Older toolchains contain reachable standard
library vulnerabilities. The repository pins Go 1.26.5 for development and
release builds through mise:

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
mise exec -- make test-container
mise exec -- make test-race
mise exec -- make integration-oss
mise exec -- make lint
mise exec -- make snapshot
~~~

## Releases

Pushing a tag matching the supported FerricStore line, such as `v0.11.4`, runs
the complete test suite and publishes compressed binaries plus checksums
through GoReleaser. The release fails before publishing if the tag, the version
in `FERRICSTORE_VERSION`, and the pinned Go SDK version do not match. See
[RELEASE.md](RELEASE.md) for the release checklist.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Security issues should follow
[SECURITY.md](SECURITY.md).

## License

Apache License 2.0. See [LICENSE](LICENSE).
