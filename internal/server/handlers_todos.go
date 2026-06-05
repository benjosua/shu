package server

import (
	"net/http"
	"time"
)

func (a *App) createTodo(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	var in struct {
		Title, Body, DueAt, RemindAt, Priority, Source string
		Tags                                           []string
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Title == "" {
		writeError(w, r, 400, "title required")
		return
	}
	state := map[string]any{"status": TodoOpen}
	if in.DueAt != "" {
		state["due_at"] = in.DueAt
	}
	if in.RemindAt != "" {
		state["remind_at"] = in.RemindAt
	}
	if in.Priority != "" {
		state["priority"] = in.Priority
	}
	if in.Source != "" {
		state["source"] = in.Source
	} else {
		state["source"] = "manual"
	}
	extID := "todo:" + randHex(12)
	var id string
	err = a.db.QueryRow(r.Context(), `insert into items(workspace_id,source_resource_id,kind,external_id,title,body,state,tags) values($1,null,'todo.item',$2,$3,$4,$5,$6) returning id::text`, ws, extID, in.Title, in.Body, mustJSON(state), mustJSON(nonNilTags(in.Tags))).Scan(&id)
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"id": id, "kind": "todo.item", "external_id": extID, "title": in.Title, "body": in.Body, "state": state, "tags": nonNilTags(in.Tags)})
}

func (a *App) listTodos(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = TodoOpen
	}
	sql := `select id::text,title,body,state,tags,created_at,updated_at from items where workspace_id=$1 and kind='todo.item'`
	args := []any{ws}
	if status != "all" {
		sql += ` and coalesce(state->>'status','open')=$2`
		args = append(args, status)
	}
	sql += ` order by coalesce((state->>'due_at')::timestamptz, created_at) asc limit 200`
	rows, err := a.db.Query(r.Context(), sql, args...)
	writeRows(w, rows, err, "id", "title", "body", "state", "tags", "created_at", "updated_at")
}

func (a *App) updateTodo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := a.requireTodo(w, r, id); !ok {
		return
	}
	var in struct {
		Title, Body, DueAt, RemindAt, Priority, Source string
		Tags                                           []string
		State                                          map[string]any
	}
	if !readJSON(w, r, &in) {
		return
	}
	state := nonNilMap(in.State)
	if in.DueAt != "" {
		state["due_at"] = in.DueAt
	}
	if in.RemindAt != "" {
		state["remind_at"] = in.RemindAt
		state["last_reminded_at"] = nil
	}
	if in.Priority != "" {
		state["priority"] = in.Priority
	}
	if in.Source != "" {
		state["source"] = in.Source
	}
	_, err := a.db.Exec(r.Context(), `update items set title=case when $2<>'' then $2 else title end, body=case when $3<>'' then $3 else body end, state=case when $4::jsonb <> '{}'::jsonb then state || $4::jsonb else state end, tags=case when $5::jsonb <> '[]'::jsonb then $5::jsonb else tags end, updated_at=now() where id=$1`, id, in.Title, in.Body, mustJSON(state), mustJSON(nonNilTags(in.Tags)))
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) completeTodo(w http.ResponseWriter, r *http.Request) {
	a.setTodoStatus(w, r, r.PathValue("id"), TodoCompleted)
}

func (a *App) reopenTodo(w http.ResponseWriter, r *http.Request) {
	a.setTodoStatus(w, r, r.PathValue("id"), TodoOpen)
}

func (a *App) setTodoStatus(w http.ResponseWriter, r *http.Request, id, status string) {
	if _, ok := a.requireTodo(w, r, id); !ok {
		return
	}
	state := map[string]any{"status": status}
	if status == TodoCompleted {
		state["completed_at"] = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := a.db.Exec(r.Context(), `update items set state=state || $2::jsonb, updated_at=now() where id=$1`, id, mustJSON(state))
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "status": status})
}

func (a *App) requireTodo(w http.ResponseWriter, r *http.Request, id string) (string, bool) {
	ws, ok := a.requireObjectRole(w, r, "items", id, RoleMember)
	if !ok {
		return "", false
	}
	var kind string
	if err := a.db.QueryRow(r.Context(), `select kind from items where id=$1`, id).Scan(&kind); err != nil || kind != "todo.item" {
		writeError(w, r, 404, "todo not found")
		return "", false
	}
	return ws, true
}

func nonNilTags(tags []string) []string {
	if tags == nil {
		return []string{}
	}
	return tags
}
