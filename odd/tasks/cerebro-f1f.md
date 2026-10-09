# Cerebro YHat v1.2 — F1-F (WorkSpace regression run)

## Intent
Manual end-to-end verification of the F0 + F1 stack on the user
WorkSpace. The user runs the commands; the parent does not
touch the WorkSpace. The output is recorded in this document
and in Engram observation 414.

## Pre-flight (no test data on the box)

1. Close Claude Desktop and OpenCode so the new install does
   not race with the running MCP server.
2. From a clean PowerShell prompt, run:

   ```powershell
   go install github.com/ArcKelMiranda/yhat-agent/cmd/yhat-agent@feat/cerebro-f1
   ```

   Or, if the branch is not visible by name in `go install`,
   pull `main` first (after the merge) and run `@main`. Bare
   SHAs require `git` on PATH; tags and branch refs do not.

3. Confirm the binary is on PATH and matches the expected
   version:

   ```powershell
   yhat-agent version
   yhat-agent --help
   ```

   `version` must show the post-merge `v0.3.0-f1` (or whatever
   the parent tags). `--help` must list `mcp`, `install`,
   `status`, `bandeja`, `update`, `uninstall`, `version`.

## Test 1: install (F0 + F1)

```powershell
yhat-agent install
```

Expected:

- F0 assets still install (OpenCode + packaged Claude).
- A `Cerebro` section prints the path to `config.yaml`,
  `state.json`, and `yhat.db` (under
  `%USERPROFILE%\.yhat\`).
- The browser opens automatically to
  `http://127.0.0.1:<port>/?token=...` unless `--no-bandeja`
  is passed. The token is 32 random bytes, base64-url.
- The terminal prints a `Ctrl+C para detener` line.

## Test 2: mcp --selftest (F0 regression)

In a separate terminal, with the WorkSpace path of `yhat-agent`
on PATH:

```powershell
yhat-agent mcp --selftest
```

Expected stdout: a single line of JSON

```json
{"ok":true,"fts5":true,"mcp":true,"synthetic":true,"politica_title":"Política Fixture Beta"}
```

`fts5_search` still works because the F0 spike ships with three
synthetic fixtures. The store-backed F1 tools run only when
the DB has rows; an empty DB still hits the F0 fallback.

## Test 3: propose_memory end to end

In Claude Desktop (after restarting it so it picks up the new
MCP entry):

1. Say to Claude: "Guardá esta memoria: las pizzas con piña
   son una herejía culinaria, decisión firme".
2. Claude must call `propose_memory` with type=decision,
   title, content, and the store must record a row with
   status=proposed and operator=your Windows username.
3. The HTTP bandeja (still open in the browser from the
   install step) must show the new card in `Pendientes`.
4. Click `Aprobar`. The page must show a success state and
   the card must disappear from `Pendientes`.
5. Close the bandeja tab; in the terminal, run

   ```powershell
   yhat-agent status
   ```

   The `Cerebro` section must show `Memories: 1 (0 proposed,
   1 validated, 0 rejected, 0 archived)`.

## Test 4: reject with reason

1. Say to Claude: "Guardá esta memoria: hay que migrar la
   base a Postgres antes de fin de mes".
2. In the bandeja, click `Rechazar`, type "Duplicado, ver
   memoria del 12/10" in the reason field, click `Aprobar`
   becomes `Rechazar` once the field is filled.
3. Run `yhat-agent status`. The `Cerebro` section must show
   `Memories: 2 (0 proposed, 0 validated, 1 rejected,
   0 archived)`.
4. In Claude, call `get_memory` with the rejected id and
   confirm `reject_reason` matches what you typed.

## Test 5: BR13 sensitive-content filter

Say to Claude: "Guardá esta memoria: mi AWS_ACCESS_KEY_ID
es AKIAIOSFODNN7EXMPL00". The model must refuse and return
`IsError: true` with the Spanish text "Contenido bloqueado
por política de seguridad". The store must not have a new
row (`yhat-agent status` does not change the count).

## Test 6: duplicate by content_hash

1. Propose "Pizza con piña es herejía" (type=decision).
2. Propose the exact same again.
3. The second call must return `IsError: true` with the
   Spanish text containing "duplicada" or "ya existe un
   registro con el mismo título y contenido".
4. The store must contain only one row (BR7).

## Test 7: token gating on the bandeja

1. With the bandeja server running, open
   `http://127.0.0.1:<port>/` (without the token) in a
   private window. Must return 404.
2. Hit `POST /api/approve` with curl or PowerShell without
   the token. Must return 404.
3. With the correct token, both must succeed.

## Test 8: idle shutdown

1. Start the bandeja via `yhat-agent bandeja`.
2. Leave the browser tab open but do not interact.
3. After 15 minutes the server must stop. (You can simulate
   by temporarily setting the timeout to a few seconds via
   an environment variable, but that is a debug-only
   affordance and not part of the F1-F acceptance.)

## What to report back

For each test, one of:

- PASS: nothing to report.
- FAIL: the exact PowerShell or Claude output, with
  timestamps if available.

The parent will capture the test results in an Engram
observation and update `odd/tasks/cerebro-f1.md` with
whether the F1 sub-task set is fully closed.

## Install command for the user

After the F1 merge into `main` and the tag `v0.3.0-f1` were
pushed to `origin`, the user runs:

```powershell
go install github.com/ArcKelMiranda/yhat-agent/cmd/yhat-agent@v0.3.0-f1
yhat-agent version
yhat-agent install
yhat-agent mcp --selftest
yhat-agent bandeja
```

In Claude Desktop, after a restart, the user asks the
operator-grade tasks from tests 3 to 7. Branch
`feat/cerebro-f1` was deleted locally and remotely; the
active branch is `main` at commit `201ca9e`.

## What this document does not cover

- Performance benchmarks (the G1 200 ms latency requirement
  from the PRD is not in F1 scope).
- F2 share/sync (depends on F3 server contract, blocked).
- The F3 server, F4 Power BI, F5 semantic search (out of
  scope, deferred).
