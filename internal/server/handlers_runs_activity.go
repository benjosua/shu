package server

import (
	"fmt"
	"net/http"
)

func (a *App) listRuns(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	status := r.URL.Query().Get("status")
	kind := r.URL.Query().Get("kind")
	sql := `select id::text,kind,subject_type,coalesce(subject_id::text,''),status,input,result,error,started_at,completed_at,created_at,updated_at from runs where workspace_id=$1`
	args := []any{ws}
	n := 2
	if status != "" {
		sql += fmt.Sprintf(" and status=$%d", n)
		args = append(args, status)
		n++
	}
	if kind != "" {
		sql += fmt.Sprintf(" and kind=$%d", n)
		args = append(args, kind)
	}
	sql += ` order by created_at desc limit 200`
	rows, err := a.db.Query(r.Context(), sql, args...)
	writeRows(w, rows, err, "id", "kind", "subject_type", "subject_id", "status", "input", "result", "error", "started_at", "completed_at", "created_at", "updated_at")
}

func (a *App) getRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ws, ok := a.requireObjectRole(w, r, "runs", id, RoleMember)
	if !ok {
		return
	}
	_ = ws
	writeRow(w, a.db.QueryRow(r.Context(), `select id::text,kind,subject_type,coalesce(subject_id::text,''),status,input,result,error,started_at,completed_at,created_at,updated_at from runs where id=$1`, id), "id", "kind", "subject_type", "subject_id", "status", "input", "result", "error", "started_at", "completed_at", "created_at", "updated_at")
}

func (a *App) listActivity(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	subjectType := r.URL.Query().Get("subject_type")
	subjectID := r.URL.Query().Get("subject_id")
	sql := `select id::text,subject_type,coalesce(subject_id::text,''),type,actor_type,coalesce(actor_id::text,''),payload,created_at from activity_events where workspace_id=$1`
	args := []any{ws}
	n := 2
	if subjectType != "" {
		sql += fmt.Sprintf(" and subject_type=$%d", n)
		args = append(args, subjectType)
		n++
	}
	if subjectID != "" {
		sql += fmt.Sprintf(" and subject_id=$%d", n)
		args = append(args, subjectID)
	}
	sql += ` order by created_at desc limit 200`
	rows, err := a.db.Query(r.Context(), sql, args...)
	writeRows(w, rows, err, "id", "subject_type", "subject_id", "type", "actor_type", "actor_id", "payload", "created_at")
}
