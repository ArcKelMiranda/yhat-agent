# Windows WorkSpace MCP registration

## Intent
User explicitly authorized adding automatic MCP registration to `yhat-agent install` for Windows WorkSpaces, keeping existing OpenCode asset installation and adding Claude Desktop. This connects the existing diagnostic F0 tool, not production capture.

## Scope and decisions
- Branch `feat/cerebro-local`; no commits, staging, push, release or real machine configuration changes from this session. Tests use temporary profiles only.
- Keep legacy Install API/assets and F0 files intact. Wire Windows CLI install to a separate MCP registration helper after legacy install succeeds; non-Windows behavior unchanged.
- OpenCode global config: current OpenCodeRoot()/opencode.json. Honor explicit OPENCODE_CONFIG when present and absolute; refuse JSONC or ambiguous json/jsonc rather than strip comments or create a shadow config. Official schema: mcp.yhat={type:local,command:[absolute executable,mcp],enabled:true}.
- Claude: discover existing conventional APPDATA/Claude directory and packaged LOCALAPPDATA/Packages/Claude_*/LocalCache/Roaming/Claude directory. Exactly one target; multiple candidates are an explicit error, none means reported skip. User confirmed package family Claude_pzs8sxrjxfjjc on profile drive D:; never hardcode drive/user or write WindowsApps.
- Register current executable absolute path (os.Executable), no binary relocation/PATH changes. Claude entry mcpServers.yhat={command:absolute,args:[mcp]}. Client registration key need not equal SDK server implementation name.
- Validate object JSON and namespace shapes, retain unrelated values with json.RawMessage, refuse conflicting existing yhat entries. Identical registration is a byte-preserving no-op with no new backup. No forced overwrite option.
- Before replacing an existing config, write a unique exact-byte backup beside it and a temporary new config; refuse symlinks/nonregular targets; close/sync before rename. Fail safely if backup or replacement fails. Report per-client paths/outcomes, not full config contents. Close clients during installation; do not claim cross-file transactionality.
- Existing status/uninstall remain legacy asset operations; document that MCP registration is separate and preserved by uninstall. No claims of full F1 lifecycle support.

## Tasks and routing
- [x] W1 (delegated writer): Implement detection, safe merge/backup/idempotence and Windows CLI wiring, focused tests and documentation. Multi-file trigger. Test-first required with observed RED/GREEN. Included in commit 288b196.
- [ ] W2 (pending, delegated verifier): Full tests, vet, Windows CGO-free build, selftest and installer tests under disposable profiles. Native assessment/review as offered; no use of expired prior consent bindings.
- [ ] W3 (pending, human environment): Run compiled installer and verify actual Claude/OpenCode startup on WorkSpace. Never infer runtime success from cross-build.

## Acceptance and checks
Preserve other servers/preferences, validate both Claude layouts, reject ambiguity/conflicts/malformed JSON/JSONC/symlinks, exact backup, repeat-install no-op, explicit executable path with spaces, non-Windows unchanged. Commands use PATH=/snap/go/current/bin:$PATH and CGO_ENABLED=0: focused tests, go test -count=1 ./..., go vet ./..., go run ./cmd/yhat-agent mcp --selftest, GOOS=windows GOARCH=amd64 go build to /tmp/yhat-workspace/yhat-agent.exe, git diff --check.

## Delivery
Forecast 400–600 authored lines for one coherent installer unit with tests/docs; advisory only, no minifying or omitted tests. Strategy ask-on-risk before any eventual oversized commit/PR, none authorized. Commit identities pending permission. Existing F0 review and WorkSpace validation remain open; prior native attempts created no lineage.

## Progress
Exploration complete; OpenCode format verified at https://opencode.ai/docs/mcp-servers/ and /docs/config/. User confirmed actual packaged Claude path. No installer source changed yet.

## Review progress
Native review (lineage review-a921ac9aaf9b4d74) ran all four lenses and closed with `correction_required` and two candidate-caused findings:

- R3-sensitive-patterns-required (BLOCKER): content-safety test in yhat-agent_test.go hard-failed when `.sensitive-content-patterns` is missing, while the same candidate gitignores that file. Corrected to skip silently when ENOENT and fail only on other read errors; no .gitignore change.
- R4-MCP-001 (CRITICAL): runMCPCommand ran the SDK with `context.Background()` so a hung client would leave the process alive forever. Corrected to a 1-hour bounded context with defer cancel.

Both fixes verified with `go vet`, `go test -count=1 ./...` and `mcp --selftest`. Budget used 12 of 200 correction lines. No other source touched. Awaiting the provider correction outcome to close or surface further defects.
