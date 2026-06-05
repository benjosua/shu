package server

import (
	"net/http"
)

func (a *App) inboxUnreadCount(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `select count(*)::int from inbox_items where workspace_id=$1 and read=false and archived=false`, ws), "count")
}

func (a *App) inboxMarkAllRead(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	_, err = a.db.Exec(r.Context(), `update inbox_items set read=true where workspace_id=$1`, ws)
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) inboxArchiveAll(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	_, err = a.db.Exec(r.Context(), `update inbox_items set archived=true where workspace_id=$1`, ws)
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) inboxArchiveRead(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	_, err = a.db.Exec(r.Context(), `update inbox_items set archived=true where workspace_id=$1 and read=true`, ws)
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
