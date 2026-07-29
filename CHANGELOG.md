# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog, and this project follows Semantic
Versioning.

## Unreleased

### Added

- Initial Go command-line scaffold.
- FerricStore Go SDK integration boundary.
- OSS username/password login with hidden terminal input, stdin support,
  profile storage, and operating-system keyring storage.
- Real protected-mode OSS login integration tests and mocked Enterprise
  authentication provider contracts.
- Docker-backed protected-mode OSS CLI integration for FQL, query plans,
  query-index status, convenience reads, and typed schedules.
- Saved-profile connection routing and the `ferric server ping` connectivity
  command.
- AWS-style ephemeral environment credentials for OSS passwords and Enterprise
  API-token providers, including bounded Docker/Kubernetes secret-file input.
- Profile list, show, use, and delete commands with dynamic shell completion.
- Connection-verified authentication status and idempotent local logout.
- Redis-shaped FerricStore helpers across strings, collections, streams,
  expiry, probabilistic structures, geospatial data, locks, rate limits, and
  other Ferric-native operations, plus a one-command escape hatch.
- JSON/stdin MULTI/EXEC transactions with cluster-slot routing and WATCH.
- FerricFlow queue commands with send/receive/ack aliases, lease and fencing
  safety, delayed work, history, statistics, and policy management.
- Workflow start, search, describe, claim, signal, transition, lineage,
  history, value, rewind, terminal, and failure commands.
- Direct bounded FQL1 query, explain/analyze, and OSS query-index status
  commands with typed parameters, file/stdin input, diagnostics, quality, and
  resource-usage output.
- Partition-scoped FQL convenience reads and workflow search predicates for
  attributes and per-state metadata.
- Workflow-owned one-shot, delayed, interval, and cron schedule management.
- FerricStore Go SDK v0.11.4 schedule contracts, including interval catch-up,
  overlap/coalescing state, and rich recurrence status output.
- Server diagnostics and configuration, cluster operations, OSS ACL
  administration, namespace/quota usage, and one-shot Pub/Sub operations.
- Workflow governance commands for approvals, circuit breakers, effects,
  budgets, and distributed limits.
- Shared auto/JSON/raw output, global network timeouts, file/stdin payloads,
  destructive-operation confirmation, examples, and dynamic completion.
- Context-aware cross-process profile locking and failure-reporting credential
  rollback for safe concurrent login and profile updates.
- Central raw-command and transaction safety classification, with explicit
  confirmation before destructive commands can open a connection.
- Compile-time validation of every advertised official SDK capability and
  explicit CLI-owned schemas for typed SDK output.
- Focused command-safety and output-contract packages that keep policy and
  presentation concerns out of command construction.
- Go 1.25.12 minimum and Go 1.26.5 release builds, excluding known reachable
  standard-library vulnerabilities in the former Go 1.24 baseline.
- Formatting, linting, unit, race, vulnerability, and CodeQL checks.
- Cross-platform release packaging for Linux, macOS, and Windows.
- A distroless, non-root Ferric utility image for Linux amd64 and arm64 with
  Docker/Kubernetes environment authentication, contract tests, and protected
  OSS integration coverage.
- GHCR release publishing with FerricStore-aligned tags, an SBOM, and build
  provenance plus a GitHub artifact attestation.
- Lockstep CLI release tags that are validated against the declared
  FerricStore OSS compatibility line and pinned Go SDK version.
