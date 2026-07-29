# Release Process

Releases are built by GitHub Actions from semantic-version tags. A CLI release
tag matches the FerricStore OSS and Go SDK release it is based on. For example,
CLI `v0.11.4` targets FerricStore OSS `v0.11.4` or newer and uses SDK `v0.11.4`.

`FERRICSTORE_VERSION` is the repository's compatibility source of truth. The
release workflow rejects a tag when that file, the pinned SDK module, and the
Git tag do not all match.

## Prepare

1. Confirm CI and security checks pass on main.
2. Update `FERRICSTORE_VERSION`, the Go SDK dependency, the pinned OSS Docker
   image digest, and compatibility documentation together when moving to a new
   FerricStore release line.
3. Update CHANGELOG.md.
4. Run the local validation, version check, and packaging snapshot:

       mise exec -- make verify
       mise exec -- make test-container
       golangci-lint run ./...
       mise exec -- ./scripts/verify-release-version.sh v0.11.4
       goreleaser release --snapshot --clean

5. Verify the generated archives in dist/.

## Publish

Create and push an annotated semantic-version tag:

~~~sh
git tag -a v0.11.4 -m "Ferric CLI v0.11.4 for FerricStore OSS v0.11.4"
git push origin v0.11.4
~~~

The release workflow reruns tests and then publishes:

- Linux binaries for amd64 and arm64
- macOS binaries for amd64 and arm64
- Windows binaries for amd64 and arm64
- source archive
- SHA-256 checksum file
- a distroless GHCR image for Linux amd64 and arm64, tagged `v0.11.4`,
  `0.11.4`, and `latest`, with an SBOM, build provenance, and a GitHub artifact
  attestation

GitHub initially creates a new container package as private. After the first
successful image publication, an organization package administrator must open
the `command-line` package settings and change its visibility to public. This
is a one-time irreversible GitHub setting; subsequent release tags update the
same public package.

If the workflow fails before publishing, fix the issue and create a new tag.
Do not move a published tag.

Release tags are intentionally one-to-one with FerricStore OSS/SDK releases.
If a CLI-only correction must ship after a tag is published, coordinate a new
FerricStore OSS/SDK patch release rather than inventing a divergent CLI tag.
