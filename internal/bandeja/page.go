// page.go contains the embedded Spanish HTML for the Bandeja page.
// No JS framework, no external CSS, no external fonts — minimal inline JS only.

package bandeja

import (
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/ArcKelMiranda/yhat-agent/internal/store"
)

// buildPage assembles the HTML page from pending and sent memories.
func buildPage(pending []store.Memory, sent []store.Memory, operator string) string {
	var sb strings.Builder
	sb.WriteString(pageHeader)
	sb.WriteString("\n<body>\n")
	sb.WriteString(pageHeading)
	sb.WriteString("\n")

	sb.WriteString("\n<section>\n<h2>Pendientes</h2>\n")
	if len(pending) == 0 {
		sb.WriteString("<p class=\"empty\">No hay memorias pendientes.</p>\n")
	} else {
		for _, mem := range pending {
			sb.WriteString(memCard(mem, "pendiente"))
		}
	}
	sb.WriteString("</section>\n")

	sb.WriteString("\n<section>\n<h2>Enviados</h2>\n")
	if len(sent) == 0 {
		sb.WriteString("<p class=\"empty\">No hay memorias enviadas.</p>\n")
	} else {
		for _, mem := range sent {
			sb.WriteString(memCard(mem, "enviado"))
		}
	}
	sb.WriteString("</section>\n")

	sb.WriteString(pageFooter)
	return sb.String()
}

// memCard renders a single memory card with action buttons.
func memCard(mem store.Memory, section string) string {
	age := formatAge(mem.CreatedAt)
	contentPreview := truncate(mem.Content, 200)
	id := template.HTMLEscapeString(mem.ID)
	title := template.HTMLEscapeString(mem.Title)
	contentEsc := template.HTMLEscapeString(contentPreview)
	typeLabel := template.HTMLEscapeString(mem.Type)

	var actions string
	if section == "pendiente" {
		actions = fmt.Sprintf(`<button onclick="doApprove('%s')">Aprobar</button>
        <button onclick="doReject('%s')">Rechazar</button>
        <button onclick="doEdit('%s', '%s', '%s')">Editar</button>`,
			id, id, id,
			strings.ReplaceAll(template.HTMLEscapeString(mem.Title), "'", "\\'"),
			strings.ReplaceAll(template.HTMLEscapeString(mem.Content), "'", "\\'"))
	}

	var reasonStr string
	if mem.RejectReason != nil && *mem.RejectReason != "" {
		reasonStr = fmt.Sprintf(`<p class="reason"><strong>Razón:</strong> %s</p>`,
			template.HTMLEscapeString(*mem.RejectReason))
	}

	return fmt.Sprintf(`<div class="card" data-id="%s">
  <div class="card-header">
    <span class="type">%s</span>
    <span class="age">%s</span>
  </div>
  <h3>%s</h3>
  <p class="content">%s</p>
  %s
  %s
</div>`,
		id,
		typeLabel,
		age,
		title,
		contentEsc,
		reasonStr,
		actions,
	)
}

// formatAge returns a human-readable age string.
func formatAge(t time.Time) string {
	age := time.Since(t)
	if age < time.Minute {
		return "ahora"
	}
	if age < time.Hour {
		mins := int(age.Minutes())
		if mins == 1 {
			return "hace 1 minuto"
		}
		return fmt.Sprintf("hace %d minutos", mins)
	}
	if age < 24*time.Hour {
		hours := int(age.Hours())
		if hours == 1 {
			return "hace 1 hora"
		}
		return fmt.Sprintf("hace %d horas", hours)
	}
	if age < 30*24*time.Hour {
		days := int(age.Hours() / 24)
		if days == 1 {
			return "hace 1 día"
		}
		return fmt.Sprintf("hace %d días", days)
	}
	return t.Format("02/01/2006")
}

// truncate cuts s to at most maxLen runes, appending "…" if truncated.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "…"
}

const pageHeader = `<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Bandeja YHat</title>
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #f5f5f5; color: #222; max-width: 900px; margin: 0 auto; padding: 20px; }
h1 { font-size: 1.5rem; margin-bottom: 20px; color: #1a1a1a; }
h2 { font-size: 1.2rem; margin: 24px 0 12px; color: #333; border-bottom: 1px solid #ddd; padding-bottom: 4px; }
section { margin-bottom: 32px; }
.card { background: #fff; border: 1px solid #e0e0e0; border-radius: 8px; padding: 16px; margin-bottom: 12px; box-shadow: 0 1px 3px rgba(0,0,0,0.06); }
.card-header { display: flex; justify-content: space-between; margin-bottom: 8px; }
.type { font-size: 0.75rem; background: #e3f2fd; color: #1565c0; padding: 2px 8px; border-radius: 4px; text-transform: uppercase; }
.age { font-size: 0.75rem; color: #888; }
.card h3 { font-size: 1rem; margin-bottom: 6px; color: #111; }
.content { font-size: 0.875rem; color: #555; line-height: 1.5; margin-bottom: 10px; word-break: break-word; }
.reason { font-size: 0.8rem; color: #c62828; background: #ffebee; border-left: 3px solid #c62828; padding: 6px 10px; margin-top: 8px; border-radius: 0 4px 4px 0; }
button { font-size: 0.8rem; padding: 5px 12px; border: 1px solid #ccc; border-radius: 4px; background: #fff; cursor: pointer; margin-right: 6px; transition: background 0.15s; }
button:hover { background: #f0f0f0; }
button.approve { background: #e8f5e9; border-color: #81c784; color: #2e7d32; }
button.approve:hover { background: #c8e6c9; }
button.reject { background: #ffebee; border-color: #e57373; color: #c62828; }
button.reject:hover { background: #ffcdd2; }
.empty { color: #888; font-style: italic; padding: 12px 0; }
#msg { position: fixed; bottom: 20px; left: 50%; transform: translateX(-50%); padding: 10px 20px; border-radius: 6px; font-size: 0.875rem; display: none; z-index: 100; }
#msg.ok { background: #e8f5e9; color: #2e7d32; border: 1px solid #81c784; display: block; }
#msg.err { background: #ffebee; color: #c62828; border: 1px solid #e57373; display: block; }
</style>
</head>`

const pageHeading = `<h1>Bandeja YHat</h1>`

const pageFooter = `<div id="msg"></div>
<script>
function showMsg(text, ok) {
  var el = document.getElementById('msg');
  el.textContent = text;
  el.className = ok ? 'ok' : 'err';
  setTimeout(function() { el.style.display = 'none'; }, 3000);
}

function fetchAPI(path, body) {
  var token = new URLSearchParams(window.location.search).get('token');
  var url = path + '?token=' + encodeURIComponent(token);
  return fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  }).then(function(r) { return r.json(); });
}

function doApprove(id) {
  fetchAPI('/api/approve', {id: id}).then(function(d) {
    if (d.ok) {
      showMsg('Aprobado', true);
      var card = document.querySelector('.card[data-id="' + id + '"]');
      if (card) setTimeout(function() { card.remove(); }, 800);
    } else {
      showMsg('Error: ' + d.error, false);
    }
  });
}

function doReject(id) {
  var reason = prompt('Razón del rechazo (opcional):');
  fetchAPI('/api/reject', {id: id, reason: reason || ''}).then(function(d) {
    if (d.ok) {
      showMsg('Rechazado', true);
      var card = document.querySelector('.card[data-id="' + id + '"]');
      if (card) setTimeout(function() { card.remove(); }, 800);
    } else {
      showMsg('Error: ' + d.error, false);
    }
  });
}

function doEdit(id, title, content) {
  var newTitle = prompt('Nuevo título:', title);
  if (newTitle === null) return;
  var newContent = prompt('Nuevo contenido:', content);
  if (newContent === null) return;
  fetchAPI('/api/edit', {id: id, title: newTitle, content: newContent}).then(function(d) {
    if (d.ok) {
      showMsg('Editado', true);
      location.reload();
    } else {
      showMsg('Error: ' + d.error, false);
    }
  });
}
</script>
</body>
</html>`
