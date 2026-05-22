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

func (a *App) getNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	uid := currentUserID(r)
	writeRowNullable(w, a.db.QueryRow(r.Context(), `select preferences,updated_at from notification_preferences where workspace_id=$1 and user_id=$2`, ws, uid), "preferences", "updated_at")
}

func (a *App) updateNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	uid := currentUserID(r)
	var in map[string]any
	if !readJSON(w, r, &in) {
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `insert into notification_preferences(workspace_id,user_id,preferences) values($1,$2,$3) on conflict(workspace_id,user_id) do update set preferences=excluded.preferences,updated_at=now() returning preferences,updated_at`, ws, uid, mustJSON(in)), "preferences", "updated_at")
}
