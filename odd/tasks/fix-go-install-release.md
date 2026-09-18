# Fix Go install and release updates

## Goal
Make `go install github.com/ArcKelMiranda/yhat-agent/cmd/yhat-agent@latest` install corrected code and make `yhat-agent update` avoid the GitHub API.

## Tasks
- [x] Replace version-specific/API update discovery with GitHub's stable `releases/latest/download` asset URLs; add reliable version reporting and tests.
- [x] Remove generated release binaries from source control, repair module metadata, and document the supported Go installation/update flow.
- [ ] Verify Linux and Windows builds, publish immutable `v0.1.3`, and confirm the Go module proxy resolves it.

## Evidence
- Writer verification: `go test ./...`, native build, and Windows cross-build passed.
- Independent verification: deterministic test suite, native/Windows builds, temporary Go install, and version/ldflags checks passed.
- Parent spot check: `go test ./... -count=1` passed.
