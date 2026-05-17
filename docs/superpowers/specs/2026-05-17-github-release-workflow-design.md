# GitHub Release Workflow Design

## Summary

Add a GitHub Actions workflow that verifies the Honch sandbox CLI, builds release binaries for multiple operating systems and architectures, and publishes them as GitHub Release assets.

The goal is to make it easy for teammates to download a ready-to-run `honch` binary without building it locally, while keeping release publishing tied to a deliberate version tag.

## Goals

- Run the sandbox CLI tests and build checks in CI before publishing.
- Produce downloadable binaries for multiple OS/arch targets.
- Publish artifacts to GitHub Releases from version tags.
- Keep a manual workflow entry point for ad hoc builds.
- Keep the workflow simple enough to maintain without a separate packaging system.

## Non-Goals

- No Homebrew formula, Scoop manifest, or package-manager integration.
- No installer UI or auto-updater.
- No release branching logic beyond version tags and manual dispatch.
- No customer-facing distribution path.

## Proposed Workflow

### Triggering

- `push` on tags matching `v*`
- `workflow_dispatch` for manual builds

### Verification Gate

Before any release artifact is published, the workflow should run in a separate
verification job:

- `go test -count=1 ./...`
- `go build ./...`

If either step fails, the workflow stops and no release assets are uploaded.

### Build Matrix

After verification passes, build release binaries for:

- `linux/amd64`
- `linux/arm64`
- `darwin/amd64`
- `darwin/arm64`

The workflow should name the outputs clearly, for example:

- `honch-linux-amd64`
- `honch-linux-arm64`
- `honch-darwin-amd64`
- `honch-darwin-arm64`

### Release Assets

Attach the following to the GitHub Release:

- the platform-specific binaries
- a checksum file for integrity verification

The release should use the tag name as the release version.

## Implementation Shape

The workflow can live in a single file:

- `.github/workflows/release.yml`

The workflow should:

1. Check out the repository.
2. Set up the Go toolchain.
3. Run verification.
4. Build the matrix binaries.
5. Generate checksums.
6. Publish the artifacts to the GitHub Release associated with the tag.

## Notes On Build Strategy

The simplest maintainable approach is to cross-compile the Go CLI from a Linux
runner using a `GOOS`/`GOARCH` matrix rather than introducing separate
platform-specific build scripts.

That keeps the release path aligned with the repository’s current Go-centric structure and avoids a second packaging layer unless the project later needs one.

If a future dependency introduces platform-specific build constraints, the workflow can move to native runners for the affected target only.

## Error Handling

- Fail fast if tests or builds fail.
- Do not publish partial releases.
- Do not overwrite an existing release asset without deliberate intent.
- Make the workflow output obvious enough that a failed build is diagnosable from GitHub Actions logs alone.

## Verification

Local verification before merging the workflow should cover:

- `go test -count=1 ./...`
- `go build ./...`

Once the workflow exists, the first tag build should be treated as the end-to-end verification of the release path.

## Open Questions

- Whether to publish the release automatically on tag push or require an approval step in GitHub Actions.
- Whether to sign artifacts later if the team wants stronger supply-chain guarantees.
- Whether the release should include only the CLI binary or also bundled helper scripts, if those ever become necessary.
