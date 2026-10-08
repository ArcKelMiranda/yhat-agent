# PRD — Cerebro YHat v1.2

Oct 7, 2026 · @Juan Carlos

## Resumen y cambios respecto a v1.1

El Cerebro YHat le da a cada operador un cerebro local en su propia computadora, donde guarda lo que descubre conversando con Claude Desktop. Cuando decide que algo vale para todos, lo comparte. El equipo lo aprueba en un repositorio central, y desde ahí vuelve a los cerebros locales de los demás. Todo lo instala y actualiza un solo binario, `yhat-agent`, reutilizando el repo que el equipo ya tiene instalado.

| Tema | v1.1 | v1.2 |
| --- | --- | --- |
| Usuario | Operador técnico en terminal | Persona no programadora en Claude Desktop |
| Entorno | Windows, macOS y Linux | Amazon WorkSpaces con Windows |
| Dónde vive el cerebro | Solo local, sync genérico en F4 | Local siempre; compartir es una orden explícita |
| Aprobación | Código de 6 dígitos por CLI | Bandeja local con botones; el agente no puede aprobar |
| Aprobación del equipo | No existía | En el repositorio central, por rol o consenso |
| Binario | `yhat-mcp` nuevo, aparte del instalador | `yhat-agent` hace todo: instalar, MCP, bandeja, actualizar |
| Ubicación | `~/.yhat-mcp/` | `~/.yhat/` (en Windows, `%USERPROFILE%\.yhat`) |
| Actualización | Descarga un candidato que se reemplaza a mano | `yhat-agent update` reemplaza, migra y reconfigura solo |
| Engram | Backend actual | Se elimina |

## Cómo funciona

Una idea recorre seis pasos, y solo sale de la computadora de su autor cuando él lo ordena. La [animación del flujo](https://claude.ai/artifact/RtZhXpid6259Ao8K1fDmjf) muestra el recorrido completo.

1. **Capturar.** María le cuenta a Claude una decisión. El agente le pregunta si la guarda y queda en su cerebro local como *Propuesta*.
2. **Aprobar en local.** María la aprueba en su bandeja, una página que se abre en su navegador. El agente propone pero no aprueba.
3. **Compartir.** María le pide al agente que la mande al equipo. Solo puede salir algo que ella ya aprobó. Viaja por la API al repositorio central.
4. **Aprobar en equipo.** En el repositorio central, el equipo la aprueba por rol o por consenso.
5. **Actualizar a todos.** Los cerebros locales de Juan y Ana bajan lo aprobado desde el repositorio central.
6. **Consultar.** Juan pregunta en su Claude Desktop. Su agente encuentra la respuesta en su propio cerebro local y le aclara que es una regla del equipo.

La búsqueda (paso 6) nunca depende de la red: siempre lee la base local, que ya tiene copia de lo del equipo.

## Usuarios, entorno y goals

Los usuarios son los \~5 operadores de YHat. No programan, trabajan con datos y Power BI, y usan Claude Desktop a diario. Cada uno tiene un Amazon WorkSpace con Windows donde ya está instalado `yhat-agent`. Nunca deberían ver una terminal, un ID ni un término técnico.

### Goals

| # | Goal | Métrica |
| --- | --- | --- |
| G1 | Buscar en el cerebro local en menos de 200 ms | p95 de `search_brain` sobre 10,000 memorias |
| G2 | Capturar sin salir de la conversación | ≥ 1 propuesta por operador por semana |
| G3 | Nada queda aprobado sin una persona | 0 memorias aprobadas sin `validated_by` |
| G4 | Compartir solo por orden explícita | 0 envíos sin una orden registrada del autor |
| G5 | Lo aprobado por el equipo llega a todos | Un cerebro local recibe una aprobación del equipo en menos de 15 min |
| G6 | Actualizar sin intervención | `yhat-agent update` deja todo listo, sin pasos manuales |

### Non-goals de v1.2

| # | No-goal | Cuándo |
| --- | --- | --- |
| N1 | Otras apps (ChatGPT, Gemini) | Después de F3 |
| N2 | Mac y Linux como objetivo de soporte (el build cruzado se mantiene) | Si el equipo cambia de entorno |
| N3 | Integración con Power BI | F4 |
| N4 | Búsqueda semántica con embeddings | F5 |
| N5 | Editar lo del equipo desde lo local | Lo del equipo es de solo lectura en cada cerebro |
| N6 | Notificaciones por Slack o email | Sin fecha |

## Arquitectura

Un solo binario, `yhat-agent`, hace todo, y vive con el cerebro en `%USERPROFILE%\.yhat`. Claude Desktop lo arranca como servidor MCP por stdio. Como su ruta nunca cambia, actualizar el binario actualiza todo sin reconfigurar la app.

| Comando | Quién lo usa | Qué hace |
| --- | --- | --- |
| `yhat-agent mcp` | Claude Desktop | Servidor MCP: buscar, proponer, compartir |
| `yhat-agent bandeja` | El operador, o el agente vía tool | Abre la bandeja de aprobación en el navegador |
| `yhat-agent update` | El operador, una vez por versión | Baja, verifica, reemplaza, migra y reconfigura |
| `yhat-agent install` | `update` lo ejecuta solo | Deja todo en el estado correcto; se puede repetir |
| `yhat-agent sync` | Arranque del MCP, en segundo plano | Envía la cola de compartidos y baja lo aprobado por el equipo |
| `yhat-agent status` | Soporte | Versión, base, apps registradas, pendientes y última sincronización |
| `yhat-agent uninstall` | Soporte | Quita lo registrado; la base se borra solo con confirmación |

### Carpeta del usuario

```
%USERPROFILE%\.yhat\
├── bin\yhat-agent.exe        el único binario
├── yhat.db                   el cerebro local (SQLite)
├── config.yaml               operador, URL del repositorio central, token
├── state.json                versión instalada y apps registradas
├── backups\                  copia de yhat.db antes de cada migración (se guardan 5)
└── logs\yhat.log             logs JSON con rotación
```

La carpeta queda accesible solo para su dueño, porque `config.yaml` guarda el token de la API. El schema SQL no vive suelto en esta carpeta: viene dentro del binario como migraciones.

### Estructura del repo `yhat-agent`

```
yhat-agent/
├── cmd/yhat-agent/main.go          comandos (se agregan mcp, bandeja, sync)
├── internal/
│   ├── home/                       rutas de ~/.yhat y permisos
│   ├── store/                      SQLite, FTS5 y migraciones
│   │   └── migrations/001_init.sql
│   ├── mcp/                        servidor, tools e instrucciones del agente
│   ├── bandeja/                    página local de aprobación (HTML embebido)
│   ├── share/                      cola de envío y descarga desde el repositorio central
│   ├── clients/                    registra el MCP en Claude Desktop (y OpenCode)
│   └── selfupdate/                 reemplazo del binario en Windows
├── install.go, update.go, …        se adaptan a ~/.yhat
├── assets/                         instrucciones reescritas, sin Engram
└── .github/workflows/              mismo release, Go actualizado
```

Se conservan la descarga con checksum, el manifest, el release multiplataforma y los tests que no dependan del texto de los assets. Se eliminan las referencias a Engram (`mem_save`, `mem_search`, `topic_key` como identificador obligatorio).

## Tools MCP y bandeja

El agente tiene seis tools y ninguna aprueba. Aprobar y rechazar solo existen como botones de la bandeja, así que la regla "solo un humano aprueba" la garantiza la estructura, no una instrucción.

| Tool | Fase | Qué hace | Devuelve |
| --- | --- | --- | --- |
| `search_brain` | F1 | Busca en lo propio y lo del equipo | Resultados con origen (mío o del equipo) y estado |
| `propose_memory` | F1 | Guarda una idea como Propuesta | Título y estado, o el aviso de algo parecido |
| `list_pending` | F1 | Lista lo que espera aprobación | Títulos y antigüedad |
| `get_memory` | F1 | Muestra una memoria completa | Contenido, contexto, origen e historial |
| `open_bandeja` | F1 | Abre la bandeja en el navegador | Confirmación de que se abrió |
| `share_memory` | F2 | Encola una memoria aprobada para enviarla al equipo | Estado del envío |

**Instrucciones del agente.** Viajan dentro del servidor MCP y las recibe Claude Desktop al conectarse. Le dicen que hable en lenguaje simple, sin IDs ni términos técnicos. También le dicen que pregunte antes de guardar, que distinga entre propuesta, aprobada por mí y del equipo, y que solo llame a `share_memory` cuando la persona lo pida en ese momento.

**Bandeja.** Es una página servida por `yhat-agent` en `127.0.0.1` con un puerto aleatorio y un token de un solo uso en la URL. Solo responde a esa computadora y se apaga sola a los 15 minutos sin uso. Muestra tarjetas con los botones Aprobar, Rechazar (con motivo opcional) y Editar, y una sección de Enviados con el estado de cada envío al equipo.

## Modelo de datos

Una sola tabla guarda lo propio y lo del equipo, separados por `origin`, así una sola búsqueda FTS5 cubre ambos. Los estados locales y los del envío al equipo van en columnas distintas, porque son dos procesos independientes. Este es `internal/store/migrations/001_init.sql`.

```sql
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE memories (
    id              TEXT PRIMARY KEY,                -- UUID; nunca se muestra al usuario
    origin          TEXT NOT NULL CHECK (origin IN ('local','team')),
    type            TEXT NOT NULL CHECK (type IN ('decision','rule','anomaly','improvement')),
    title           TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    content         TEXT NOT NULL CHECK (length(content) BETWEEN 1 AND 10000),
    context         TEXT,
    content_hash    TEXT NOT NULL,                   -- SHA-256 de título + contenido normalizados
    author          TEXT NOT NULL,                   -- de config.yaml, o el autor que manda el repositorio central
    -- ciclo local (solo origin = 'local')
    status          TEXT NOT NULL DEFAULT 'proposed'
                    CHECK (status IN ('proposed','validated','rejected','archived')),
    validated_by    TEXT,
    validated_at    TIMESTAMP,
    reject_reason   TEXT,
    -- envío al equipo (solo origin = 'local')
    share_status    TEXT NOT NULL DEFAULT 'none'
                    CHECK (share_status IN ('none','queued','sent','accepted','rejected')),
    shared_at       TIMESTAMP,
    -- copia del repositorio central (solo origin = 'team')
    remote_id       TEXT UNIQUE,
    team_status     TEXT CHECK (team_status IN ('approved','retired')),
    team_updated_at TIMESTAMP,
    is_example      INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (status <> 'validated' OR (validated_by IS NOT NULL AND validated_at IS NOT NULL)),
    CHECK (share_status = 'none' OR status IN ('validated','archived')),  -- solo se comparte lo aprobado
    CHECK (origin = 'local' OR remote_id IS NOT NULL)
);

CREATE INDEX idx_memories_origin_status ON memories(origin, status, type);
CREATE UNIQUE INDEX idx_memories_live_hash
    ON memories(origin, content_hash) WHERE status IN ('proposed','validated');

CREATE VIRTUAL TABLE memories_fts USING fts5(
    title, content, context,
    content='memories', content_rowid='rowid',
    tokenize='unicode61 remove_diacritics 2'           -- "politica" encuentra "política"
);

CREATE TRIGGER memories_ai AFTER INSERT ON memories BEGIN
    INSERT INTO memories_fts(rowid, title, content, context)
    VALUES (new.rowid, new.title, new.content, new.context);
END;
CREATE TRIGGER memories_ad AFTER DELETE ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, title, content, context)
    VALUES ('delete', old.rowid, old.title, old.content, old.context);
END;
CREATE TRIGGER memories_au AFTER UPDATE OF title, content, context ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, title, content, context)
    VALUES ('delete', old.rowid, old.title, old.content, old.context);
    INSERT INTO memories_fts(rowid, title, content, context)
    VALUES (new.rowid, new.title, new.content, new.context);
END;

-- envíos pendientes; sobrevive a reinicios y a cortes de red
CREATE TABLE upload_queue (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    memory_id     TEXT NOT NULL REFERENCES memories(id),
    ordered_by    TEXT NOT NULL,                     -- quién dio la orden (BR10)
    ordered_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    attempts      INTEGER NOT NULL DEFAULT 0,
    last_error    TEXT,
    next_retry_at TIMESTAMP
);

-- punto desde donde seguir bajando lo del equipo
CREATE TABLE sync_state (
    key   TEXT PRIMARY KEY,                          -- 'pull_cursor', 'last_pull_at', 'last_push_at'
    value TEXT NOT NULL
);
```

Los tipos se guardan en inglés y se muestran como Decisión, Regla, Algo raro y Mejora. Respecto a v1.1 desaparecen `confirm_challenges` (la bandeja reemplaza el código) y `sessions`, que no aportaba a ninguna tool. El trigger de UPDATE solo se dispara cuando cambia el texto, así los cambios de estado no reescriben el índice.

## Reglas de negocio

| Regla | Descripción | Dónde se garantiza |
| --- | --- | --- |
| BR1 | Toda memoria local nace como Propuesta. | Default del schema |
| BR2 | Solo una persona aprueba o rechaza, y solo desde su bandeja. | No existe una tool que apruebe |
| BR3 | Una memoria aprobada siempre tiene quién y cuándo. | `CHECK` del schema |
| BR4 | `is_example = 1` se excluye de búsquedas y reportes. | Filtro en `store` |
| BR5 | Título de 1 a 200 caracteres, contenido de 1 a 10,000. | `CHECK` del schema |
| BR6 | La búsqueda ignora mayúsculas y tildes. No hay stemming en v1.2. | Tokenizer FTS5 |
| BR7 | Si ya existe algo igual, no se duplica: el agente lo muestra y pregunta. Si es parecido, avisa sin bloquear. | Índice único por hash y búsqueda de similares |
| BR8 | El autor y el aprobador salen de `config.yaml`, nunca de un parámetro del agente. | `mcp` ignora esos campos |
| BR9 | Solo se comparte lo que su autor ya aprobó en local. | `CHECK` del schema |
| BR10 | `share_memory` solo se usa por una orden explícita del usuario en esa conversación. Cada envío queda registrado y visible en la bandeja. | Instrucciones del agente y registro en `upload_queue` |
| BR11 | Lo del equipo es de solo lectura en cada cerebro local. | `store` rechaza cambios con `origin = 'team'` |
| BR12 | Las respuestas siempre dicen si algo es mío (propuesta o aprobado) o del equipo. | `search_brain` devuelve origen y estado |
| BR13 | Antes de guardar, se rechaza contenido que parezca una contraseña, token o API key. | Validación en `propose_memory` |

BR10 es la única regla que depende de que el agente obedezca. Si se quiere garantizarla por estructura, el envío puede requerir un click en la bandeja, igual que la aprobación. Queda como open question.

## Instalación y actualización

`yhat-agent update` deja todo listo en un solo comando. Si algo falla a mitad de camino, vuelve a la versión anterior con la base intacta.

1. **Consultar** la última versión en GitHub Releases (mecanismo actual, sin token).
2. **Descargar y verificar** el `.exe` contra su SHA-256 (mecanismo actual).
3. **Reemplazar el binario.** Windows no deja sobrescribir un ejecutable en uso, pero sí renombrarlo. Se renombra `yhat-agent.exe` a `yhat-agent.old.exe`, el nuevo toma su lugar y el viejo se borra en el próximo arranque.
4. **Ejecutar `install` con el binario nuevo**, para que la versión nueva sea la que configura:
   1. Copia `yhat.db` a `backups\` y aplica las migraciones pendientes.
   2. Registra o actualiza la entrada `yhat` en `%APPDATA%\Claude\claude_desktop_config.json`, sin tocar los otros servidores y con backup previo del archivo.
   3. Si encuentra OpenCode, actualiza también su registro.
   4. Guarda versión y apps registradas en `state.json`.
5. **Autoprueba.** Ejecuta `yhat-agent mcp --selftest`, que abre la base, lista las tools y hace una búsqueda. Si falla, restaura el binario y la base del backup.
6. **Avisar** en lenguaje simple: "Listo, actualizado a la versión X. Cerrá y abrí Claude Desktop." Claude Desktop sigue usando el proceso viejo hasta que se reinicia.

### Primera migración desde la versión instalada hoy

La versión actual no sabe reemplazarse: deja un archivo candidato al lado del binario. Por eso el primer salto es manual, una sola vez, y es corto:

1. Correr `yhat-agent update` como siempre. Deja el candidato y muestra su ruta.
2. Ejecutar ese candidato con `install`. La versión nueva se copia a `%USERPROFILE%\.yhat\bin`, agrega esa carpeta al PATH del usuario, registra el MCP en Claude Desktop y avisa si el binario viejo sigue en otra ruta del PATH.

Desde ahí, cada `yhat-agent update` hace todo solo. También `install` retira los assets que dejaron las versiones con Engram, siempre que su hash coincida con el del manifest. Si alguien los modificó, los deja y avisa.

### Notas de Amazon WorkSpaces

- Si los WorkSpaces usan la configuración estándar, el perfil del usuario (con `.yhat` y la configuración de Claude Desktop) vive en el volumen que persiste entre reinicios. Hay que confirmarlo con quien administra los WorkSpaces.
- AppLocker, Defender o SmartScreen pueden bloquear un `.exe` sin firma que corre desde el perfil del usuario. Se valida en el spike de F0. Si bloquean, hay que firmar el binario o pedir una excepción de ruta.
- El repositorio central puede vivir en la misma red de AWS que los WorkSpaces, sin exponerse a internet.

## Fases y estimación

El lado del cliente (F0 a F2) suma 23 días hábiles. El servidor del repositorio central queda por estimar hasta decidir si es `yhat-knowledge` o un servicio nuevo.

| Fase | Scope | Criterio de done | Días |
| --- | --- | --- | --- |
| F0 | Spike en un WorkSpace real | Las 5 preguntas del spike respondidas | 1.5 |
| F1 | Cerebro local: MCP, base, bandeja, install y update completos | Un operador captura, aprueba y consulta sin ayuda | 15.5 |
| F2 | Compartir y actualizar desde el repositorio central (lado cliente) | Una memoria va de un WorkSpace a otro | 6 |
| F3 | Aprobación del equipo por rol o consenso (lado servidor) | El equipo aprueba y el resultado baja a todos | Por estimar |
| F4 | Power BI | Métricas consultables desde Claude Desktop | Por estimar |
| F5 | Búsqueda semántica | Búsqueda por concepto, no solo por palabras | Por estimar |

### Spike F0

1. ¿`modernc.org/sqlite` (Go puro) trae FTS5 y compila para Windows con `CGO_ENABLED=0`?
2. ¿Qué versión mínima de Go pide el SDK de MCP?
3. ¿Claude Desktop en el WorkSpace arranca el MCP desde `%USERPROFILE%\.yhat\bin`?
4. ¿AppLocker, Defender o SmartScreen bloquean el `.exe`?
5. ¿Se puede renombrar el `.exe` mientras Claude Desktop lo está usando?

### Desglose de F1 (15.5 días)

| Tarea | Días |
| --- | --- |
| `home`: rutas, `config.yaml`, `state.json`, permisos | 1 |
| `store`: SQLite, migraciones, FTS5, backups | 2.5 |
| `mcp`: servidor, 5 tools e instrucciones | 2.5 |
| `bandeja`: página local, token, aprobar, rechazar y editar | 2 |
| `install`: Claude Desktop, PATH, retiro de assets de Engram | 1.5 |
| `selfupdate`: reemplazo en Windows, autoprueba y rollback | 2 |
| Assets reescritos sin Engram | 0.5 |
| Tests: adaptar los existentes y agregar los nuevos | 2.5 |
| Prueba con un operador en su WorkSpace | 1 |

### Desglose de F2, lado cliente (6 días)

| Tarea | Días |
| --- | --- |
| `share_memory`, cola y reintentos | 1.5 |
| Bajada de lo aprobado con cursor | 1.5 |
| Contrato de la API con el repositorio central | 1 |
| Sección Enviados en la bandeja | 1 |
| Tests | 1 |

### Checklist F1

- [ ] `yhat-agent update` desde la versión nueva reemplaza, migra, se autoprueba y avisa, sin pasos manuales.
- [ ] Si la autoprueba falla, vuelve al binario y la base anteriores.
- [ ] La primera migración desde la versión con Engram funciona con los dos pasos documentados.
- [ ] `install` se puede correr dos veces seguidas sin cambiar nada la segunda vez.
- [ ] `install` agrega su entrada en la config de Claude Desktop sin tocar otros servidores.
- [ ] Claude Desktop muestra las 5 tools de F1 al reiniciarse.
- [ ] "Guardalo" en una conversación crea una Propuesta; el agente pregunta antes de guardar.
- [ ] El agente no tiene forma de aprobar: solo la bandeja cambia el estado.
- [ ] La bandeja solo responde en `127.0.0.1` y con el token de la URL.
- [ ] Buscar "politica" encuentra "política" en menos de 200 ms sobre 10,000 memorias.
- [ ] Proponer dos veces lo mismo no duplica; el agente muestra el existente.
- [ ] Un texto con algo parecido a una contraseña o token no se guarda.
- [ ] El agente nunca muestra IDs ni términos técnicos en sus respuestas.
- [ ] `go test ./...` pasa y el release genera el `.exe` con su checksum.

## Riesgos y open questions

| # | Riesgo | Mitigación |
| --- | --- | --- |
| R1 | La política de seguridad del WorkSpace bloquea el `.exe` | Probarlo en el spike F0; firmar el binario o pedir excepción de ruta |
| R2 | El agente comparte sin una orden clara del usuario | BR10, registro de cada envío en la bandeja; opción de exigir click (pregunta 2) |
| R3 | Una migración corrompe la base | Backup antes de cada migración, autoprueba y rollback automático |
| R4 | Una actualización de Claude Desktop cambia el formato de su config | `install` valida el JSON antes de escribir y guarda backup del archivo |
| R5 | Los operadores no proponen memorias | El agente propone en el momento; métrica G2 por operador |
| R6 | La búsqueda sin stemming pierde variantes ("aprobar" y "aprobación") | Prefijos en la consulta; embeddings en F5 |
| R7 | Alguien quedó sin hacer la primera migración manual | Hasta que migre, sigue con la versión vieja sin romperse; seguimiento uno a uno, son 5 personas |

### Open questions

1. **Repositorio central.** ¿Es `yhat-knowledge` (ya existe en el servidor, con UI) o un servicio nuevo? Define el contrato de la API de F2 y la estimación de F3.
2. **Compartir.** ¿Alcanza con la orden en el chat (BR10) o el envío también requiere un click en la bandeja?
3. **Aprobación del equipo.** ¿Por rol (un responsable) o por consenso (cuántos de los 5)? Se decide antes de F3.
4. **Rechazo del equipo.** Si el equipo rechaza algo compartido, ¿el autor lo ve en su bandeja con el motivo? Propuesta: sí.
5. **Identidad.** ¿De dónde sale el nombre del operador en `config.yaml`: lo escribe en la instalación o se toma del usuario de Windows?
6. **`yhat-mcp-server`.** Existe un servidor MCP en Node en el servidor de YHat, que usa `mssql`. ¿Qué hace y cómo convive con este?
7. **Frecuencia de la bajada.** ¿Al abrir Claude Desktop y cada 15 minutos alcanza? Propuesta: sí.

### Siguiente paso

Resolver la pregunta 1, correr el spike F0 en un WorkSpace y abrir la branch `feat/cerebro-local` en `yhat-agent`.
