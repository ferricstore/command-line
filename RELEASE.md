# Release Process

Releases are built by GitHub Actions from semantic-version tags. A CLI release
tag matches the FerricStore OSS and Go SDK release it is based on. For example,
CLI `v0.11.11` targets FerricStore OSS `v0.11.11` or newer and uses SDK `v0.11.11`.

`FERRICSTORE_VERSION` is the CLI compatibility source of truth. The release
workflow rejects a tag when that file, the pinned SDK module, and the Git tag
do not all match. `FERRICSTORE_IMAGE_VERSION` and
`FERRICSTORE_IMAGE_DIGEST` independently pin the newer OSS server image used
by integration tests, so server patch validation does not force an unrelated
CLI or SDK release.

Repository administrators must protect `v*` tags from updates/deletion and
protect the `release` environment with required reviewers. The workflow also
rejects tagged commits that are not reachable from `main`, reruns the complete
CI and security workflows for the tag, and refuses to move an existing version
image to a different source revision.

## Prepare

1. Confirm CI and security checks pass on main.
2. Update `FERRICSTORE_VERSION`, the Go SDK dependency, and compatibility
   documentation together when moving the CLI to a new FerricStore release
   line. Update `FERRICSTORE_IMAGE_VERSION` and `FERRICSTORE_IMAGE_DIGEST`
   together whenever integration tests move to a newer compatible OSS server.
3. Update CHANGELOG.md.
4. Run the local validation, version check, and packaging snapshot:

       mise exec -- make verify
       mise exec -- make test-container
       mise exec -- make integration-oss
       mise exec -- make integration-http
       golangci-lint run ./...
       mise exec -- ./scripts/verify-release-version.sh v0.11.11
       goreleaser release --snapshot --clean

5. Verify the generated archives in dist/.

## Publish

Create and push an annotated semantic-version tag:

~~~sh
git tag -a v0.11.11 -m "Ferric CLI v0.11.11 for FerricStore OSS v0.11.11"
git push origin v0.11.11
~~~

The release workflow reruns tests and then publishes:

- Linux binaries for amd64 and arm64
- macOS binaries for amd64 and arm64
- Windows binaries for amd64 and arm64
- source archive
- SHA-256 checksum file
- a distroless GHCR image for Linux amd64 and arm64, tagged `v0.11.11`,
  `0.11.11`, and `latest`, with an SBOM, build provenance, and a GitHub artifact
  attestation

GitHub initially creates a new container package as private. After the first
successful image publication, an organization package administrator must open
the `command-line` package settings and change its visibility to public. This
is a one-time irreversible GitHub setting; subsequent release tags update the
same public package.

If validation fails before any artifact is published, fix the issue before
creating the immutable release tag. If an upload job fails after another
artifact has published, rerun the failed job for the same tag; do not create,
move, or delete a replacement tag. Release jobs derive timestamps from the
tagged commit, so a retry uses the same build inputs.

Release tags are intentionally one-to-one with FerricStore OSS/SDK releases.
If a CLI-only correction must ship after a tag is published, coordinate a new
FerricStore OSS/SDK patch release rather than inventing a divergent CLI tag.
