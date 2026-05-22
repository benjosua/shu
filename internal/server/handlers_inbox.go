package server

import (
	"net/http"
)

func (a *App) listInbox(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	rows, err := a.db.Query(r.Context(), `select id::text,type,severity,title,body,read,archived,created_at from inbox_items where workspace_id=$1 and archived=false order by created_at desc`, ws)
	writeRows(w, rows, err, "id", "type", "severity", "title", "body", "read", "archived", "created_at")
}

func (a *App) inboxRead(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "inbox_items", r.PathValue("id"), RoleMember); !ok {
		return
	}
	if _, err := a.db.Exec(r.Context(), `update inbox_items set read=true where id=$1`, r.PathValue("id")); err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]string{"ok": "true"})
}

func (a *App) inboxArchive(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "inbox_items", r.PathValue("id"), RoleMember); !ok {
		return
	}
	if _, err := a.db.Exec(r.Context(), `update inbox_items set archived=true where id=$1`, r.PathValue("id")); err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]string{"ok": "true"})
}
