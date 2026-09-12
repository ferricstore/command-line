# Authentication Architecture

The CLI treats authentication as a provider-owned login followed by shared,
transactional persistence.

## Methods

| Method | Deployment | Credential presented to the provider |
|---|---|---|
| password | OSS | ACL username and password |
| enterprise-sso | Enterprise human | Time-limited Platform `fsp_user_` token |
| enterprise-api-token | Enterprise machine | Service-account API token |

All three methods are implemented. Enterprise methods call the versioned
Platform credential broker and then authenticate a real native SDK connection
with the returned short-lived credential.

## OSS Login

The password provider constructs the official Go SDK client, connects to the
configured endpoint, and executes PING. It uses `WithNativeCredentials` for
`ferric://` and `ferrics://`, and `WithHTTPBasicAuth` for `https://`. State is
persisted only after successful authentication. Plaintext `http://` is rejected
before network access when username/password credentials are present.

The profile document contains the endpoint, method, username, and optional
non-secret CA certificate path. It never contains the password. Passwords are
stored under the corresponding profile in the operating-system keyring.

If profile persistence fails after the credential was written, the login
service restores the previous keyring value or removes the newly written value.

## Command Connections

Login validates and saves authentication state; it does not keep a TCP socket
or background CLI process alive. Each one-shot command performs this lifecycle:

1. Resolve an explicit profile, direct environment credentials, or the
   selected/default saved profile.
2. Read the secret from the selected environment source or operating-system
   keyring without combining the two.
3. Route the non-secret connection metadata and secret to its provider.
4. Open an SDK client, execute the command, and close the client.

The selected connection is resolved in this order:

1. Explicit `--profile` override.
2. Complete direct environment credentials.
3. `FERRIC_PROFILE` environment variable.
4. Connection selected by `ferric profile use`.
5. The `default` connection for a new installation.

## Environment Credentials

OSS direct credentials require `FERRIC_URL`, `FERRIC_USERNAME`, and exactly one
of `FERRIC_PASSWORD` or `FERRIC_PASSWORD_FILE`. HTTPS endpoints may also set
`FERRIC_CA_CERT_FILE`. Enterprise machine credentials
require `FERRIC_CONTROL_URL`, `FERRIC_ORGANIZATION`, `FERRIC_CLUSTER`, and
exactly one of `FERRIC_API_TOKEN` or `FERRIC_API_TOKEN_FILE`.

Setting any variable in one set activates strict validation of that complete
set. OSS and Enterprise environment variables cannot be combined, inline and
file secrets cannot both be set, and missing values are errors. The CLI does
not fall back to a profile for missing metadata or to the keyring for a missing
secret. Secret files are read only when a network command or authentication
status check resolves that source, are limited to 64 KiB, and may end with one
newline.

Environment credentials are ephemeral and are never persisted. The
`ferric auth status` command verifies them through a real PING and reports
`Source: environment`. Without an explicit `--profile`, `ferric auth logout`
tells the caller to unset the environment variables and leaves saved
credentials untouched.

`ferric profile list` and `ferric profile show` display only non-secret
metadata. Deleting a profile removes both its metadata and keyring credential.
If metadata deletion fails, the CLI restores the credential.

`ferric auth status` opens a real authenticated SDK connection and executes
PING. A credential is not reported as authenticated merely because it exists
locally. For a saved profile, `ferric auth logout` removes the selected local
keyring credential and keeps the profile metadata. OSS logout does not revoke
or change the ACL password on the server; an administrator controls server-side
password rotation and user removal.

For an OSS password profile, the SDK configures username/password
authentication on every native connection or HTTPS request. FerricStore
remains the authorization authority and checks the authenticated user's ACL
for each command; the CLI does not cache permission decisions.

HTTPS uses system roots by default. A saved `--ca-cert` path or ephemeral
`FERRIC_CA_CERT_FILE` appends a private PEM CA while retaining hostname
verification and TLS 1.2 or newer. HTTP redirects are not followed, preventing
credentials from crossing origins, and there is no insecure
certificate-verification flag. Standard `HTTP_PROXY`,
`HTTPS_PROXY`, and `NO_PROXY` behavior comes from the Go transport.
Relative `--ca-cert` paths are converted to absolute paths before profile
validation and persistence. `FERRIC_CA_CERT_FILE` must be absolute because
environment credentials are resolved independently for each invocation.

Enterprise connection providers use the same boundary. They exchange a stored
time-limited user token or service-account token for a temporary cluster
credential before returning the SDK client. The Platform token is never sent
to the FerricStore endpoint. Each one-shot command performs a new exchange, so
native credentials are not persisted locally.

Long-lived commands such as a future interactive shell or watch operation may
keep one SDK client for their process lifetime, reconnecting and refreshing
credentials through the provider as needed. That is separate from login and
does not require a permanent CLI daemon.

## Enterprise Boundary

Enterprise providers implement the same login provider interface and return:

- validated profile metadata
- authenticated principal
- Platform user or service-account token to store in the keyring

Login requires `--control-url`, `--cluster`, and `--token-stdin`; organization
slug or ID is optional but is checked when supplied. The control URL must use
HTTPS except for an explicit loopback development endpoint. Redirects,
credential-bearing URLs, oversized responses, invalid native endpoints, and
expired credential responses fail closed.

The command-line repository unit-tests routing, secret persistence, exchange
validation, and token non-disclosure. Platform owns the Docker-backed end-to-end
test against the packaged Enterprise server, including authentication and
expiry on an already-open native connection.

No public test should require Enterprise source code, credentials, or a live
Enterprise control plane.

## Test Ownership

| Behavior | command-line repository | Platform / Enterprise integration |
|---|---|---|
| OSS native username/password | Real protected OSS server | Optional |
| OSS HTTPS username/password and custom CA | Real TLS HTTP listener | Optional |
| Per-command HTTP ACL denial | Full and PING-only users | Optional |
| Invalid OSS password | Real protected OSS server | Optional |
| Saved OSS login reused by a new connection | Real native and HTTPS servers | Optional |
| Profile selection, deletion, and completion | Unit tests | Optional |
| Auth status and local logout | Real OSS connection and unit tests | Optional |
| Provider routing and persistence | Unit tests with mocks | Optional |
| Human token exchange | HTTP contract and provider unit tests | Packaged Enterprise integration through Platform |
| Service-account token exchange | HTTP contract and provider unit tests | Platform broker/RBAC integration |
| Temporary native credential expiry | Unit contract | Packaged Enterprise integration through Platform |
| Environment OSS credentials and secret files | Real protected OSS server | Optional |
| Environment API-token routing | Provider and environment tests | Platform broker/RBAC integration |

The native OSS integration exercises username/password over `ferric://`. The HTTP
integration separately exercises username/password over a real `https://`
listener, a private CA, saved-profile reuse after a working-directory change,
HTTP/2 negotiation, invalid credentials, and a user whose ACL permits `PING`
but denies `SET`.

Cloud integration deliberately runs the Enterprise package; standalone OSS is
not a Platform cloud target. The CLI does not impose TLS policy on direct native
OSS connections; deployments choose between `ferric://` and `ferrics://`.
