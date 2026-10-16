# Cerebro YHat v1.2 — estado actual (2026-10-16)

## Ramas

- `main` = `origin/main` = `c54642c` (F2 LocalDirTransport). Tags
  publicados hasta `v0.3.0-f1`.
- `feat/cerebro-f2-http`: F2 HTTPTransport + arreglo de la bandeja
  F1-D + reescritura del flag global `--central-token`. Commiteado,
  sin push y sin merge a `main`.

## Fases

| Fase | Estado |
|---|---|
| F0 prototipo MCP/FTS5 + instalador | ✅ done |
| F0-2 revisión nativa | ⏳ pendiente (sesión nueva) |
| F1-A..E store, tools MCP, BR13, install, bandeja | ✅ done (bandeja arreglada recién en `feat/cerebro-f2-http`) |
| F1-F pasada manual en WorkSpace | ⏳ pendiente; usar un binario con el arreglo de bandeja |
| MSI WiX 3.14 | ✅ done |
| F2 LocalDirTransport | ✅ done (`c54642c`) |
| F2 HTTPTransport | ✅ commiteado en la rama, falta push/merge |
| F3 yhat-central | ⚠️ código fuente perdido (ver abajo) |
| F4 Power BI / F5 búsqueda semántica | ⏳ diferidos hasta uso real |

## Bug de la bandeja F1-D (resuelto)

La nota anterior ("3 tests rojos desde F2") era incorrecta: fallaban
14 de 26 tests desde el commit original de F1-D (`226d1ee`), y los
botones Aprobar / Rechazar / Editar no funcionaban en ningún build.
Causa: rutas registradas con barra final (`/api/approve/`) mientras la
página hace `POST /api/approve` → redirect 301 → el navegador lo
reintenta como GET → 405. Además el helper `baseURL` de los tests
generaba `//api/...`. Detalle en `odd/tasks/cerebro-f1.md`.

## F3 perdido

`/tmp/yhat-central` (commit `ea5481f`, 20 tests) ya no existe: `/tmp`
se limpió y nunca se pusheó. Sobreviven solo el binario
`/tmp/yhat-central-bin/yhat-central` y `mock_f3_server.go`. Opciones:
recuperarlo de otra copia o reconstruirlo desde `docs/central-api.md`.
Cuando exista, guardarlo fuera de `/tmp` y pushearlo a
`github.com/ArcKelMiranda/yhat-central`.

## Verificación (2026-10-16, `feat/cerebro-f2-http`)

- `CGO_ENABLED=0 go test -count=1 ./...`: 9 paquetes ok (bandeja 26/26).
- `CGO_ENABLED=0 go vet ./...`: limpio.
- `yhat-agent mcp --selftest`: `ok:true`, "Política Fixture Beta",
  sin ruido de flags en stderr.
- Build Windows CGO-free (`-trimpath`): SHA-256
  `722f1b0fee7ba784f1ee74c9a45591147339c877c9c7651d56dd07c89d1c246b`.
- Toolchain: Go 1.23 en `~/go-new/bin` (no está en el PATH por defecto).

## Próximos pasos

1. Push de `feat/cerebro-f2-http` y merge a `main`.
2. Recuperar o reconstruir F3 y pushearlo a su repo.
3. F2 e2e contra el F3 real.
4. F1-F en el WorkSpace.
5. Release `v0.4.0` + MSI + tag.
6. F0-2 revisión nativa.
