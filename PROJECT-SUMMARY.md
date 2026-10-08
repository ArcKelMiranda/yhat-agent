# yhat-agent — Resumen del Proyecto

## 1. Propósito y rol en el ecosistema

`yhat-agent` es un binario Go CLI que instala, verifica y mantiene los activos
embebidos del agente y skill de captura de conocimiento YHat para OpenCode.
Los activos van compilados en el binario (`go:embed`) y se despliegan en el
directorio de configuración de OpenCode bajo seguimiento de manifiesto local.

**Rol funcional:** capturar conocimiento operativo duradero (reglas de negocio,
decisiones, patrones, mapeos, definiciones de entidad, excepciones, procesos)
durante sesiones de IA con captura silenciosa, validación de calidad, puntuación
de cumplimiento y preguntas priorizadas por el humano, sin interrumpir el flujo
de trabajo. Engram provee infraestructura de staging; `yhat-knowledge` es la
fuente de verdad externa confirmada por un humano.

---

## 2. Baseline frente a avances entregados (commit 2a69174+)

| Área | Baseline | Avance actual |
|------|----------|---------------|
| Timestamp de captura | Generado por el modelo | Shell UTC (`date -u` / PowerShell) |
| Preguntas antes de ofrecer | Ningún sweep previo | Sweep de lectura obligatorio |
| Orden de enriquecimiento | Solo preguntas locales | Compliance (menor) → edad → impacto |
| Cierre de sesión | Aging check básico | Sweep + hechos sin capturar + aging |
| Auditoría multi-proyecto | Proyecto activo únicamente | `all_projects: true` + proyecto activo |
| Métricas de auditoría | Sin exclusión clara | `is_example: true` excluido explícitamente |
| Test de contenido seguro | Escaneo básico | Paths SO, usuario activo, patrones locales, SQL real |
| Tests de permisos | Sin skip as root | `skipIfRoot()` en tests de escritura atómica |

---

## 3. CLI Go — Comandos y comportamiento

```
yhat-agent install        # Copia activos embebidos al directorio OpenCode
yhat-agent status         # Informe del manifiesto: archivos, hashes, drift, candidatos
yhat-agent update         # Descarga desde GitHub Releases; SHA-256 verificado;
                          # candidato lado a lado; nunca sobrescribe el binario en uso
yhat-agent version        # ldflags → VCS revision → dev
yhat-agent uninstall --yes  # Elimina archivos no modificados; requiere --yes
```

**install/update:** escritura exclusiva atómica (`O_CREATE|O_EXCL`, sin
ventana TOCTOU); install: drift → `.yhat-agent-new`, rechaza symlinks y
directorios, registra `ContentHash` e `InstalledHash`, idempotente. update:
descarga desde `releases/latest/download/` sin token; SHA-256 verificado antes
de escribir; candidato lado a lado (`<nombre>.yhat-agent-new`); usuario aplica
renombrando.

**status:** drift (hash actual ≠ embebido → `StateDrift`), drift vs. install original
(hash actual ≠ `InstalledHash` → `StateDrift`), candidatos por sufijo
`.yhat-agent-new`. Estados: `missing`, `installed`, `drift`, `candidate`, `unknown`.

**uninstall:** requiere `--yes`; verifica integridad (hash ≠ `InstalledHash` →
error, no elimina); conserva manifiesto si algo queda; lo elimina si todo se
removió; limpia directorios padre vacíos.

**version:** `-ldflags '-X ...'` → `go mod version` → revisión VCS → `dev`;
`BuildInfo` indica cambios sin commit (`vcs.modified: true`).

---

## 4. Agente y skill embebidos de OpenCode

Archivos compilados en el binario (`go:embed assets/*`):

```
assets/agents/yhat-memory-capture.md
assets/skills/yhat-memory-capture/SKILL.md
```

Versión vigente: **1.3.0** — activada cuando el agente `yhat-memory-capture`
se selecciona para una sesión. El skill documenta el protocolo en cinco fases:

| Fase | Cuándo | Qué hace |
|------|--------|----------|
| 0. Verificación inicial | Al activar (no en bootstrap) | Engram disponible, búsquedas parciales, `mem_update`, proyecto activo, duplicados |
| 1. Captura silenciosa | Inmediata al surgir conocimiento | Hygiene gate → búsqueda duplicados → timestamp UTC shell → `mem_save` atómico → 1 línea de confirmación |
| 2. Cola de enriquecimiento | Fondo, no bloqueante | Evalúa gaps; auto-enriquece desde contexto de sesión; encola como `yhat.assum.*` |
| 3. Preguntas controladas | Pausa natural, explícito, bloqueo real, cierre | Máximo 5 preguntas ordenadas por compliance; primera obligatoria fija |
| 4. Cierre de sesión | Al cerrar | Sweep lectura → hechos sin capturar → aging check → muestra cola |

---

## 5. Modelo de conocimiento YHat

```
Engram  = staging (propuesto)
yhat-knowledge = fuente de verdad externa confirmada por humano
```

El agente **nunca** confirma registros ni designa fuente de verdad. Todo
comienza como `status: proposed`; solo un humano establece `status: confirmed`,
`validated_by` y `validated_at`.

```
type: yhat-knowledge
scope: project
project: proyecto de sesión activo (PI_SESSION_PROJECT; no hard-codeado)
topic_key: yhat.{type}.{domain}.{concept}
```

---

## 6. Hygiene gate

Ocho criterios (`pass` / `warn` / `fail`): topic key válido, evidencia
presente, contenido ≥50 caracteres, sin secretos, tipo apropiado, dotted keys
extraídas, **línea literal `tags:`**, sin paths locales ni nombres de usuario.

---

## 7. Puntuación de cumplimiento (compliance) — cinco checks binarios

| # | Verificación | Condición de aprobación |
|---|-------------|--------------------------|
| 1 | Topic key válido | Coincide con `yhat.{type}.{domain}.{concept}` |
| 2 | Evidencia | Campo `evidence` presente y no nulo |
| 3 | Status | Campo `status` tiene valor |
| 4 | Línea `tags:` | Línea literal `tags:` en el cuerpo del contenido |
| 5 | Metadatos completos | `validated_by`, `captured_at`, y (`evidence` o `related_topic`) |

**Fórmula:** `compliance_score = 100 × passed_checks / 5` → 0%, 20%, 40%, 60%, 80%, 100%.
`confidence` (0.0–1.0) es **independiente**; no afecta el score.
**Estados de cobertura:** `sin registros` · `registrado sin validar` · `validado`.

---

## 8. Modo bootstrap

Activa cuando: Engram vacío, usuario pide `bootstrap yhat`, o proporciona
fuentes explícitas. En bootstrap: resumen inicial omitido · `bootstrap_mode: true`
· hygiene gate relajado (`warn` vs. `fail`) · prompts para conocimiento
fundacional · guardado en lotes por dominio.

---

## 9. Cola de enriquecimiento

Preguntas persistidas como `yhat.assum.*` con `status: pending-enrichment`.
Ordenadas por: compliance más bajo → edad más antigua → impacto de negocio.

**Auto-enriquecimiento:** antes de encolar, intenta responder desde código,
consultas o archivos de la sesión. Si encuentra evidencia, actualiza el registro
(subiendo `confidence` máximo a 0.85 sin confirmación humana); fuente:
`ai-agent`; nunca marca `status: confirmed`.

---

## 10. Conocimiento adyacente, aging y revisión rápida

**Adyacente** (`adjacent_topics`): mismo dominio/concepto diferente,
upstream/downstream.

**Aging:** usuario lo marca, `review_after` excedido 30+ días, o
conocimiento nuevo lo contradice. Verificado al inicio y cierre de sesión.

**Revisión rápida** (`"rapid review"`, `"revisá rápido"`): lotes de registros
con compliance, coverage, hygiene, confidence, evidencia, topics adyacentes.
Respuestas: `[A]pprove` / `[D]eprecate` / `[S]kip` / `[E]dit`.

---

## 11. Modo auditoría

Activado con: `"auditá yhat"`, `"yhat audit"`. **Solo lectura.**

Ejecuta: `mem_search({ all_projects: true, type: "yhat-knowledge" })` y
`mem_search({ project: activeProject })`; lista todos los proyectos
encontrados + proyecto activo; excluye `is_example: true` de todas las métricas.

**Informe:** proyectos inspeccionados, cobertura por tipo y dominio,
promedio de compliance, estados de cobertura, aging, cola, baja confianza,
evidencia faltante, knowledge adyacente, gaps, top 10 por compliance.

---

## 12. Ejemplos ficticios (EJEMPLO-NO-REAL)

Usados **solo** en documentación. Nunca se capturan ni se incluyen en gaps
ni en preguntas de enriquecimiento.

```
EntidadA, EntidadB, EntidadC    SISTEMAA, SISTEMAB, SISTEMAC
COD-001, COD-002, COD-003       SUC-A001, SUC-B002
usuario-a@ejemplo.com            usuario-b@ejemplo.com
```

Todo ejemplo ficticio incluye: `EJEMPLO-NO-REAL` + `is_example: true`.

---

## 13. Estado actual de git

**Último commit publicado:** `2a69174 feat(yhat): strengthen staged knowledge capture`

**Cambios locales sin commit** (verificados con `git status`): `.gitignore`,
`assets/agents/yhat-memory-capture.md`, `assets/skills/yhat-memory-capture/SKILL.md`,
`yhat-agent_test.go`.

**Archivo no rastreado** (verificado con `git status`): `PROJECT-SUMMARY.md`
(este documento).

**No se ejecutaron hoy:** verificaciones históricas de Go, prueba live de
Engram, ni pruebas de aceptación interactiva de OpenCode. **No se declara:**
existencia de nueva release ni resultado de revisión RDD vigente.

---

## 14. Elementos pendientes de resolver

1. Siete operaciones `mem_update` y dos registros de cola requieren IDs de
   registro, contenido original y confianzas acordadas (no proporcionados).
2. La sección de estado de revisión RDD de la versión anterior (SHA256,
   linaje, reviewer requerido) proviene de documentación temporal; se
   requiere verificación de autoridad de revisión para confirmar vigencia.
3. Confirmación explícita antes de crear commit; push o release: solo tras
   confirmación explícita.

---

## 15. Enlaces de evidencia

| Artefacto | Ruta | Descripción |
|-----------|------|-------------|
| Arquitectura (JSON) | `docs/architecture/yhat-agent.architecture.json` | Fuente Archify con conexiones |
| Arquitectura (HTML) | `docs/architecture/yhat-agent.architecture.html` | Diagrama autocontenido |
| Script de generación | `docs/architecture/generate.mjs` | Generación determinista |
| NOTICE | `docs/architecture/NOTICE.md` | Upstream licenses |
| Commit de entrega | `2a69174` | feat(yhat): strengthen staged knowledge capture |
