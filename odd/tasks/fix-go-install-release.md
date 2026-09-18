# Fix Go install and release updates

## Goal
Make `go install github.com/ArcKelMiranda/yhat-agent/cmd/yhat-agent@latest` install corrected code and make `yhat-agent update` avoid the GitHub API.

## Tasks
- [x] Replace version-specific/API update discovery with GitHub's stable `releases/latest/download` asset URLs; add reliable version reporting and tests.
- [x] Remove generated release binaries from source control, repair module metadata, and document the supported Go installation/update flow.
- [x] Verify cross-platform builds, publish a new immutable release, and confirm the Go module proxy resolves it.

## Evidence
- Work-unit commits: `0051f40` (release-safe updater), `2d11d66` (Go module version reporting).
- Writer verification: `go test ./...`, native build, and Windows cross-build passed.
- Independent verification: deterministic test suite, native/Windows builds, temporary Go install, and version/ldflags checks passed.
- Parent spot check: `go test ./... -count=1` passed.
- `v0.1.3` repaired Go installation/update; `v0.1.4` additionally fixed module version reporting without moving any published tag.
- Final published verification: normal `GOPROXY=https://proxy.golang.org,direct` resolved `v0.1.4` to `2d11d66f098ee9d28192cad55dead5017409741a`; `go install ...@latest`, `version`, `--version`, and `update` all passed.
- Release: https://github.com/ArcKelMiranda/yhat-agent/releases/tag/v0.1.4
