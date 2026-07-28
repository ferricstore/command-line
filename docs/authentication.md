# Authentication Architecture

The CLI treats authentication as a provider-owned login followed by shared,
transactional persistence.

## Methods

| Method | Deployment | Credential presented to the provider |
|---|---|---|
| password | OSS | ACL username and password |
| enterprise-sso | Enterprise human | Renewable SSO session |
| enterprise-api-token | Enterprise machine | Service-account API token |

Password login is implemented in this repository. Enterprise methods are
represented by provider contracts but do not invent or depend on an unfinished
Enterprise HTTP API.

## OSS Login

The password provider constructs the official Go SDK client with
WithNativeCredentials, connects to the configured ferric:// or ferrics:// URL,
and executes PING. State is persisted only after successful authentication.

The profile document contains the endpoint, method, and username. It never
contains the password. Passwords are stored under the corresponding profile in
the operating-system keyring.

If profile persistence fails after the credential was written, the login
service restores the previous keyring value or removes the newly written value.

## Command Connections

Login validates and saves authentication state; it does not keep a TCP socket
or background CLI process alive. Each one-shot command performs this lifecycle:

1. Load the selected non-secret profile.
2. Read its secret from the operating-system keyring.
3. Route the profile and secret to its connection provider.
4. Open an SDK client, execute the command, and close the client.

The selected connection is resolved in this order:

1. Explicit `--profile` override.
2. `FERRIC_PROFILE` environment variable.
3. Connection selected by `ferric profile use`.
4. The `default` connection for a new installation.

`ferric profile list` and `ferric profile show` display only non-secret
metadata. Deleting a profile removes both its metadata and keyring credential.
If metadata deletion fails, the CLI restores the credential.

`ferric auth status` opens a real authenticated SDK connection and executes
PING. A stored credential is not reported as authenticated merely because it
exists locally. `ferric auth logout` removes the selected local keyring
credential and keeps the profile metadata. OSS logout does not revoke or change
the ACL password on the server; an administrator controls server-side password
rotation and user removal.

For an OSS password profile, the SDK configures username/password
authentication on every TCP connection it creates. A reconnect during the
same command therefore authenticates again automatically.

Enterprise connection providers use the same boundary. They may exchange a
stored renewable SSO credential or API token for a temporary cluster
credential before returning the SDK client. The command itself does not need
to know which authentication method produced the connection.

Long-lived commands such as a future interactive shell or watch operation may
keep one SDK client for their process lifetime, reconnecting and refreshing
credentials through the provider as needed. That is separate from login and
does not require a permanent CLI daemon.

## Enterprise Boundary

Enterprise providers implement the same login provider interface and return:

- validated profile metadata
- authenticated principal
- renewable credential to store in the keyring

Mock SSO and API-token providers verify this contract in the public repository.
The Enterprise repository owns black-box tests for browser/device login, token
exchange, expiry, refresh, revocation, and real cluster authentication.

No public test should require Enterprise source code, credentials, or a live
Enterprise control plane.

## Test Ownership

| Behavior | command-line repository | Enterprise repository |
|---|---|---|
| OSS username/password | Real protected OSS server | Optional |
| Invalid OSS password | Real protected OSS server | Optional |
| Saved OSS login reused by a new connection | Real protected OSS server | Optional |
| Profile selection, deletion, and completion | Unit tests | Optional |
| Auth status and local logout | Real OSS connection and unit tests | Optional |
| Provider routing and persistence | Unit tests with mocks | Optional |
| SSO/device authorization | Mock only | Real integration |
| API-token exchange | Mock only | Real integration |
| Temporary cluster-token refresh | Mock only | Real integration |

The OSS integration deliberately exercises username/password over ferric://.
The CLI does not impose TLS policy; deployments choose between ferric:// and
ferrics://.
