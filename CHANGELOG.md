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
- Saved-profile connection routing and the `ferric server ping` connectivity
  command.
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
- Workflow-owned one-shot, delayed, interval, and cron schedule management.
- Server diagnostics and configuration, cluster operations, OSS ACL
  administration, namespace/quota usage, and one-shot Pub/Sub operations.
- Workflow governance commands for approvals, circuit breakers, effects,
  budgets, and distributed limits.
- Shared auto/JSON/raw output, global network timeouts, file/stdin payloads,
  destructive-operation confirmation, examples, and dynamic completion.
- Formatting, linting, unit, race, vulnerability, and CodeQL checks.
- Cross-platform release packaging for Linux, macOS, and Windows.
