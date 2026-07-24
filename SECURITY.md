# Security

## Supported Versions

Security fixes are provided for the latest released major version.

## Reporting a Vulnerability

Do not open a public issue for a vulnerability.

Use the repository's private
[security advisory form](https://github.com/ferricstore/command-line/security/advisories/new).
Include the affected version, reproduction steps, impact, and a suggested fix
when available.

## Credential Safety

FerricStore credentials must not be written to logs, command output, shell
history, process arguments, or committed configuration. Prefer environment
variables, standard input, or an operating-system credential store for future
authentication commands.

Use ferrics:// for TLS whenever credentials cross an untrusted network.
