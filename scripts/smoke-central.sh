#!/usr/bin/env bash
# End-to-end validation of the yhat-agent <-> Yhat-YK central channel.
#
# It exercises the real path, not a mock: propose (MCP) -> approve (bandeja) ->
# share -> sync (HTTPS push) -> approve in YK -> sync (pull) -> verify in the
# local store. It also checks that a second sync does not duplicate anything.
#
# Usage:
#   scripts/smoke-central.sh [--keep]
#
#   --keep   Do not reject the smoke-test memory in YK when the run finishes.
#
# Environment:
#   YHAT_HOME            Agent home (default: ~/.yhat).
#   YHAT_REVIEWER_TOKEN  Reviewer token of the YK central service. Without it
#                        the pull half of the round trip is skipped.
#   YHAT_AGENT_BIN       Agent binary (default: yhat-agent from PATH).
#
# Exit code 0 when every executed check passed.

set -uo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

KEEP=0
[[ "${1:-}" == "--keep" ]] && KEEP=1

YHAT_HOME="${YHAT_HOME:-$HOME/.yhat}"
CONFIG="$YHAT_HOME/config.yaml"
DB="$YHAT_HOME/yhat.db"
REVIEWER_TOKEN="${YHAT_REVIEWER_TOKEN:-}"
PY="${PYTHON:-python3}"

fail=0
pass() { printf '  \033[32mPASS\033[0m %s\n' "$1"; }
failed() { printf '  \033[31mFAIL\033[0m %s\n' "$1"; fail=1; }
skip() { printf '  \033[33mSKIP\033[0m %s\n' "$1"; }
step() { printf '\n==> %s\n' "$1"; }

# cfg_get <key> -> value of a flat key in config.yaml ("" when absent).
cfg_get() {
  sed -n "s/^$1:[[:space:]]*//p" "$CONFIG" 2>/dev/null \
    | head -1 | sed 's/^[[:space:]]*//; s/[[:space:]]*$//; s/^"\(.*\)"$/\1/'
}

step "1. Configuración del agente"
[[ -f "$CONFIG" ]] || { failed "no existe $CONFIG"; exit 1; }
CENTRAL_URL="$(cfg_get central_url)"
CENTRAL_TOKEN="$(cfg_get central_token)"
CA_FILE="$(cfg_get central_ca_file)"

if [[ -n "$CENTRAL_URL" && "$CENTRAL_URL" != "https://centro.example.invalid" ]]; then
  pass "central_url = $CENTRAL_URL"
else
  failed "central_url vacío o placeholder"
fi
if [[ -n "$CENTRAL_TOKEN" ]]; then
  pass "central_token configurado"
else
  failed "central_token vacío"
fi
if [[ -n "$CA_FILE" && -f "$CA_FILE" ]]; then
  pass "central_ca_file = $CA_FILE"
else
  failed "central_ca_file vacío o inexistente"
fi
[[ "$fail" -eq 0 ]] || exit 1

BASE="${CENTRAL_URL%/}"

step "2. TLS y salud del servicio central"
if health="$(curl -sS --cacert "$CA_FILE" --max-time 15 "$BASE/healthz" 2>&1)" \
  && [[ "$health" == "ok" ]]; then
  pass "GET /healthz -> ok"
else
  failed "GET /healthz falló: $health"
fi

step "3. La CA configurada sirve al certificado del servicio"
issuer="$(openssl s_client -connect "${BASE#https://}" -servername "${BASE#https://}" \
  </dev/null 2>/dev/null | openssl x509 -noout -issuer 2>/dev/null)"
ca_subject="$(openssl x509 -in "$CA_FILE" -noout -subject 2>/dev/null)"
if [[ -n "$issuer" && "$issuer" == *"${ca_subject#subject=}"* ]]; then
  pass "el certificado del servicio lo firma la CA configurada"
else
  failed "el emisor del servicio ($issuer) no coincide con la CA ($ca_subject)"
fi

step "4. Autenticación del agente"
code="$(curl -sS --cacert "$CA_FILE" -o /tmp/yhat_smoke_team.json -w '%{http_code}' \
  --max-time 15 -H "Authorization: Bearer $CENTRAL_TOKEN" "$BASE/api/team")"
if [[ "$code" == "200" ]]; then
  pass "GET /api/team -> 200 (token válido)"
else
  failed "GET /api/team -> $code (revisa el token)"
fi

step "5. Ronda completa: propuesta -> aprobación -> push"
TITLE="smoke-central $(date +%s)"
PAYLOAD="$(cat <<JSON
{"id":"","origin":"local","type":"observation","title":"$TITLE","content":"Memoria de prueba del validador smoke-central.","context":"smoke-central","author":"smoke-test","created_at":"$(date -u +%Y-%m-%dT%H:%M:%SZ)"}
JSON
)"

proposed="$("$PY" scripts/mcp_call.py propose_memory \
  type=observation title="$TITLE" \
  content="Memoria de prueba del validador smoke-central." \
  context="smoke-central" 2>&1)"
MEM_ID="$(printf '%s' "$proposed" | "$PY" -c 'import json,sys; print(json.load(sys.stdin)["id"])' 2>/dev/null)"
if [[ -n "$MEM_ID" ]]; then
  pass "propuesta creada: $MEM_ID"
else
  failed "no se pudo proponer la memoria: $proposed"
  exit 1
fi

# The bandeja is the only path that moves a memory to 'validated'.
bandeja_log="$(mktemp)"
yhat-agent bandeja --idle 3m >"$bandeja_log" 2>&1 &
bandeja_pid=$!
for _ in $(seq 1 20); do
  grep -q 'http://' "$bandeja_log" && break
  sleep 0.3
done
BANDEJA_URL="$(grep -o 'http://[^ ]*' "$bandeja_log" | head -1)"
BANDEJA_HOST="$(printf '%s' "$BANDEJA_URL" | sed 's|\(http://[^/?]*\).*|\1|')"
BANDEJA_TOKEN="$(printf '%s' "$BANDEJA_URL" | grep -o 'token=[^&]*' | head -1)"
approve="$(curl -sS -X POST "$BANDEJA_HOST/api/approve?$BANDEJA_TOKEN" \
  -H 'Content-Type: application/json' -d "{\"id\":\"$MEM_ID\"}" 2>&1)"
kill "$bandeja_pid" 2>/dev/null
rm -f "$bandeja_log"
if printf '%s' "$approve" | grep -q '"ok":true'; then
  pass "memoria validada en la bandeja"
else
  failed "no se pudo validar la memoria: $approve"
fi

shared="$("$PY" scripts/mcp_call.py share_memory id="$MEM_ID" 2>&1)"
if printf '%s' "$shared" | grep -q '"share_status": "queued"'; then
  pass "memoria encolada para compartir"
else
  failed "no se pudo encolar la memoria: $shared"
fi

push="$(yhat-agent sync 2>&1)"
if printf '%s' "$push" | grep -qE '^1 sent'; then
  pass "sync envió la memoria: $push"
else
  failed "sync no envió la memoria: $push"
fi

sent_state="$(sqlite3 "$DB" "SELECT share_status FROM memories WHERE id='$MEM_ID';" 2>/dev/null)"
if [[ "$sent_state" == "sent" ]]; then
  pass "el store local marca la memoria como enviada"
else
  failed "share_status local = '$sent_state' (se esperaba 'sent')"
fi

# Server-side proof: the central service already knows this id.
dup_code="$(curl -sS --cacert "$CA_FILE" -o /dev/null -w '%{http_code}' --max-time 15 \
  -X POST "$BASE/api/memories" -H "Authorization: Bearer $CENTRAL_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"id\":\"$MEM_ID\",\"origin\":\"local\",\"type\":\"observation\",\"title\":\"$TITLE\",\"content\":\"Memoria de prueba del validador smoke-central.\",\"author\":\"smoke-test\",\"created_at\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}")"
if [[ "$dup_code" == "409" ]]; then
  pass "el servicio central ya tiene la memoria (409 en el reenvío)"
else
  failed "el reenvío devolvió $dup_code (se esperaba 409: la memoria ya está en el servidor)"
fi

step "6. Pull: aprobación en YK y vuelta al store local"
if [[ -z "$REVIEWER_TOKEN" ]]; then
  skip "sin YHAT_REVIEWER_TOKEN no se puede aprobar en YK (el pull queda sin verificar)"
else
  review="$(curl -sS --cacert "$CA_FILE" --max-time 15 \
    -X POST "$BASE/api/review/$MEM_ID" \
    -H "Authorization: Bearer $REVIEWER_TOKEN" -H 'Content-Type: application/json' \
    -d '{"decision":"approved","note":"smoke-central"}')"
  if printf '%s' "$review" | grep -q '"status":"approved"'; then
    pass "memoria aprobada en YK"
  else
    failed "no se pudo aprobar en YK: $review"
  fi

  pull="$(yhat-agent sync 2>&1)"
  if printf '%s' "$pull" | grep -q 'pulled 1'; then
    pass "sync trajo la memoria del equipo: $pull"
  else
    failed "el pull no trajo la memoria: $pull"
  fi

  team_rows="$(sqlite3 "$DB" "SELECT COUNT(*) FROM memories WHERE origin='team' AND remote_id='$MEM_ID';" 2>/dev/null)"
  if [[ "$team_rows" == "1" ]]; then
    pass "el store local tiene la copia de equipo con su remote_id"
  else
    failed "copias de equipo en el store local = $team_rows (se esperaba 1)"
  fi

  again="$(yhat-agent sync 2>&1)"
  rows_after="$(sqlite3 "$DB" "SELECT COUNT(*) FROM memories WHERE origin='team' AND remote_id='$MEM_ID';" 2>/dev/null)"
  if [[ "$rows_after" == "1" ]]; then
    pass "un segundo sync no duplica nada: $again"
  else
    failed "el segundo sync duplicó la memoria (filas = $rows_after)"
  fi
fi

step "7. Limpieza"
if [[ "$KEEP" -eq 1 ]]; then
  skip "memoria conservada (--keep): $MEM_ID"
elif [[ -n "$REVIEWER_TOKEN" ]]; then
  curl -sS --cacert "$CA_FILE" --max-time 15 -X POST "$BASE/api/review/$MEM_ID" \
    -H "Authorization: Bearer $REVIEWER_TOKEN" -H 'Content-Type: application/json' \
    -d '{"decision":"rejected","note":"smoke-central: limpieza"}' >/dev/null 2>&1
  pass "memoria de prueba rechazada en YK (sale de la lista del equipo)"
else
  skip "no se puede limpiar sin YHAT_REVIEWER_TOKEN: $MEM_ID"
fi

printf '\n'
if [[ "$fail" -eq 0 ]]; then
  printf '\033[32mTodo el canal funciona.\033[0m\n'
  exit 0
fi
printf '\033[31mHay checks rotos.\033[0m\n'
exit 1
