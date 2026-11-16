#!/usr/bin/env bash
# Local CI for yhat-agent. Read-only: it never modifies or deletes files.
#
# Usage: scripts/ci-local.sh [--no-race]
set -uo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

RACE="-race"
[[ "${1:-}" == "--no-race" ]] && RACE=""

fail=0
step() { printf '\n==> %s\n' "$1"; }
run() {
  if ! "$@"; then
    echo "FAILED: $*"
    fail=1
  fi
}

step "gofmt"
unformatted="$(gofmt -l $(git ls-files '*.go'))"
if [[ -n "$unformatted" ]]; then
  echo "Files need gofmt:"
  echo "$unformatted"
  fail=1
fi

step "go vet"
run go vet ./...

step "go mod verify"
run go mod verify

step "go mod tidy is a no-op"
run go mod tidy -diff

step "cross-platform build (same targets as the release workflow)"
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
  goos="${target%/*}"
  goarch="${target#*/}"
  echo "build ${goos}/${goarch}"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" run go build -o /dev/null ./cmd/yhat-agent
done

step "go test ${RACE}"
# shellcheck disable=SC2086
run go test ${RACE} -count=1 ./...

echo
if [[ $fail -ne 0 ]]; then
  echo "CI local: FAILED"
  exit 1
fi
echo "CI local: OK"
