# Release Process

Releases are built by GitHub Actions from semantic-version tags.

## Prepare

1. Confirm CI and security checks pass on main.
2. Update CHANGELOG.md.
3. Run the local validation and packaging snapshot:

       mise exec -- make verify
       golangci-lint run ./...
       goreleaser release --snapshot --clean

4. Verify the generated archives in dist/.

## Publish

Create and push an annotated semantic-version tag:

~~~sh
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
~~~

The release workflow reruns tests and then publishes:

- Linux binaries for amd64 and arm64
- macOS binaries for amd64 and arm64
- Windows binaries for amd64 and arm64
- source archive
- SHA-256 checksum file

If the workflow fails before publishing, fix the issue and create a new tag.
Do not move a published tag.
