package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (a *App) registerExecutor(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Workspace, DaemonID, Mode string
		Executors                 []struct{ Name, Provider, Version string }
	}
	if !readJSON(w, r, &in) {
		return
	}
	ws, err := a.workspaceID(r.Context(), in.Workspace)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	if err := a.requireWorkspaceRole(r, ws, RoleMember); err != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	mode := in.Mode
	if mode == "" {
		mode = "local"
	}
	out := make([]map[string]string, 0, len(in.Executors))
	for _, ex := range in.Executors {
		name := ex.Name
		if name == "" {
			name = ex.Provider
		}
		var id string
		err := a.db.QueryRow(r.Context(), `insert into executors(workspace_id,daemon_id,name,mode,provider,status,version,last_seen_at,updated_at)
values($1,$2,$3,$4,$5,'online',$6,now(),now())
on conflict(workspace_id,daemon_id,provider) do update set name=excluded.name, mode=excluded.mode, status='online', version=excluded.version, last_seen_at=now(), updated_at=now()
returning id::text`, ws, in.DaemonID, name, mode, ex.Provider, ex.Version).Scan(&id)
		if err != nil {
			writeError(w, r, 500, err.Error())
			return
		}
		out = append(out, map[string]string{"id": id, "name": name, "provider": ex.Provider, "mode": mode})
	}
	a.publish(r.Context(), Event{Type: "executor.registered", WorkspaceID: ws, TS: time.Now()})
	writeJSON(w, map[string]any{"executors": out})
}

func (a *App) deregisterExecutor(w http.ResponseWriter, r *http.Request) {
	var in struct{ ExecutorIDs []string }
	if !readJSON(w, r, &in) {
		return
	}
	for _, id := range in.ExecutorIDs {
		ws, err := a.executorWorkspace(r.Context(), id)
		if err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
			writeError(w, r, 403, "forbidden")
			return
		}
	}
	if _, err := a.db.Exec(r.Context(), `update executors set status='offline', updated_at=now() where id::text=any($1)`, in.ExecutorIDs); err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) executorHeartbeat(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ExecutorID string `json:"executor_id"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	ws, err := a.executorWorkspace(r.Context(), in.ExecutorID)
	if err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	var id string
	err = a.db.QueryRow(r.Context(), `update executors set status='online', last_seen_at=now(), updated_at=now() where id=$1 returning id::text`, in.ExecutorID).Scan(&id)
	if err != nil {
		writeError(w, r, 404, "executor_gone")
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (a *App) listExecutors(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	rows, err := a.db.Query(r.Context(), `select id::text,name,mode,provider,status,version,last_seen_at from executors where workspace_id=$1 order by name`, ws)
	writeRows(w, rows, err, "id", "name", "mode", "provider", "status", "version", "last_seen_at")
}

func (a *App) createResource(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	var in struct {
		Kind, Locator string
		Metadata      map[string]any
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Kind == "" || in.Locator == "" {
		writeError(w, r, 400, "kind and locator required")
		return
	}
	row := a.db.QueryRow(r.Context(), `insert into resources(workspace_id,kind,locator,metadata) values($1,$2,$3,$4) returning id::text,kind,locator,metadata,created_at`, ws, in.Kind, in.Locator, mustJSON(in.Metadata))
	writeRow(w, row, "id", "kind", "locator", "metadata", "created_at")
}

func (a *App) listResources(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	rows, err := a.db.Query(r.Context(), `select id::text,kind,locator,metadata,created_at from resources where workspace_id=$1 order by created_at desc`, ws)
	writeRows(w, rows, err, "id", "kind", "locator", "metadata", "created_at")
}

func (a *App) createWork(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	var in struct {
		Kind, Title, Prompt, Provider, ResourceID, ParentID string
		Priority                                            int
		Policy                                              map[string]any
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Kind == "" {
		in.Kind = "work"
	}
	if in.Provider == "" {
		in.Provider = "codex"
	}
	executorID, err := a.pickExecutor(r.Context(), ws, in.Provider, in.ResourceID)
	if err != nil {
		writeError(w, r, 409, err.Error())
		return
	}
	row := a.db.QueryRow(r.Context(), `insert into work_items(workspace_id,kind,parent_id,title,prompt,resource_id,policy,provider,executor_id,priority)
values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) returning id::text,kind,title,status,executor_id::text,created_at`, ws, in.Kind, nullUUID(in.ParentID), in.Title, in.Prompt, nullUUID(in.ResourceID), mustJSON(in.Policy), in.Provider, executorID, in.Priority)
	var id, kind, title, status, exID string
	var created any
	if err := row.Scan(&id, &kind, &title, &status, &exID, &created); err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	a.publish(r.Context(), Event{Type: "work.created", WorkspaceID: ws, ExecutorID: exID, Payload: map[string]string{"work_id": id}, TS: time.Now()})
	writeJSON(w, map[string]any{"id": id, "kind": kind, "title": title, "status": status, "executor_id": exID, "created_at": created})
}

func (a *App) listWork(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	rows, err := a.db.Query(r.Context(), `select id::text,kind,title,provider,status,coalesce(executor_id::text,''),priority,created_at,completed_at from work_items where workspace_id=$1 order by created_at desc limit 200`, ws)
	writeRows(w, rows, err, "id", "kind", "title", "provider", "status", "executor_id", "priority", "created_at", "completed_at")
}

func (a *App) getWork(w http.ResponseWriter, r *http.Request) {
	if ws, err := a.workWorkspace(r.Context(), r.PathValue("id")); err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `select id::text,kind,coalesce(parent_id::text,''),title,prompt,provider,status,coalesce(resource_id::text,''),policy,coalesce(executor_id::text,''),result,error,created_at,started_at,completed_at from work_items where id=$1`, r.PathValue("id")), "id", "kind", "parent_id", "title", "prompt", "provider", "status", "resource_id", "policy", "executor_id", "result", "error", "created_at", "started_at", "completed_at")
}

func (a *App) claimWork(w http.ResponseWriter, r *http.Request) {
	executorID := r.PathValue("id")
	ws, err := a.executorWorkspace(r.Context(), executorID)
	if err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	row := a.db.QueryRow(r.Context(), `
with claimed as (
  update work_items set status='dispatched', dispatched_at=now()
  where id=(
    select id from work_items where status='queued' and executor_id=$1 order by priority desc, created_at asc for update skip locked limit 1
  ) returning *
)
select c.id::text,c.workspace_id::text,c.title,c.prompt,c.provider,c.executor_id::text,e.mode,coalesce(r.kind,''),coalesce(r.locator,''),c.policy
from claimed c
join executors e on e.id=c.executor_id
left join resources r on r.id=c.resource_id`, executorID)
	var id, workWS, title, prompt, provider, exID, mode, resourceKind, resourceLocator string
	var policy []byte
	if err := row.Scan(&id, &workWS, &title, &prompt, &provider, &exID, &mode, &resourceKind, &resourceLocator, &policy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, map[string]any{})
			return
		}
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{
		"id": id, "workspace_id": workWS, "title": title, "body": prompt,
		"agent_id": exID, "executor_id": exID, "executor_mode": mode,
		"resource": map[string]string{"kind": resourceKind, "locator": resourceLocator},
		"policy":   json.RawMessage(policy),
		"agent":    map[string]any{"id": exID, "name": provider, "provider": provider, "instructions": "", "model": "", "custom_env": map[string]string{}, "custom_args": []string{}},
	})
}

func (a *App) startWork(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct {
		ExecutorID string `json:"executor_id"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	ws, err := a.requireWorkExecutor(r.Context(), id, in.ExecutorID)
	if err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	if err := a.db.QueryRow(r.Context(), `update work_items set status='running', started_at=now() where id=$1 and executor_id=$2 and status='dispatched' returning workspace_id::text`, id, in.ExecutorID).Scan(&ws); err != nil {
		writeError(w, r, 409, err.Error())
		return
	}
	a.publish(r.Context(), Event{Type: "work.running", WorkspaceID: ws, Payload: map[string]string{"work_id": id}, TS: time.Now()})
	writeJSON(w, map[string]string{"status": "running"})
}

func (a *App) addArtifact(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct {
		Type       string
		ExecutorID string `json:"executor_id"`
		Data       map[string]any
	}
	if !readJSON(w, r, &in) {
		return
	}
	ws, err := a.requireWorkExecutor(r.Context(), id, in.ExecutorID)
	if err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	if in.Type == "" {
		in.Type = "log"
	}
	row := a.db.QueryRow(r.Context(), `insert into artifacts(workspace_id,work_id,type,data) values($1,$2,$3,$4) returning id::text,type,created_at`, ws, id, in.Type, mustJSON(in.Data))
	writeRow(w, row, "id", "type", "created_at")
}

func (a *App) listArtifacts(w http.ResponseWriter, r *http.Request) {
	if ws, err := a.workWorkspace(r.Context(), r.PathValue("id")); err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	rows, err := a.db.Query(r.Context(), `select id::text,type,data,created_at from artifacts where work_id=$1 order by created_at`, r.PathValue("id"))
	writeRows(w, rows, err, "id", "type", "data", "created_at")
}

func (a *App) finishWork(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	status := strings.TrimPrefix(r.URL.Path, "/api/daemon/work/"+id+"/")
	if status != "complete" && status != "fail" {
		writeError(w, r, 400, "invalid finish route")
		return
	}
	dbStatus := "completed"
	if status == "fail" {
		dbStatus = "failed"
	}
	var in struct {
		Result, Error string
		ExecutorID    string `json:"executor_id"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	ws, err := a.requireWorkExecutor(r.Context(), id, in.ExecutorID)
	if err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	if err := a.db.QueryRow(r.Context(), `update work_items set status=$2,result=$3,error=$4,completed_at=now() where id=$1 and executor_id=$5 and status in ('dispatched','running') returning workspace_id::text`, id, dbStatus, in.Result, in.Error, in.ExecutorID).Scan(&ws); err != nil {
		writeError(w, r, 409, err.Error())
		return
	}
	a.publish(r.Context(), Event{Type: "work." + dbStatus, WorkspaceID: ws, Payload: map[string]string{"work_id": id}, TS: time.Now()})
	writeJSON(w, map[string]string{"status": dbStatus})
}

func (a *App) workStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	executorID := r.URL.Query().Get("executor_id")
	ws, err := a.requireWorkExecutor(r.Context(), id, executorID)
	if err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `select status from work_items where id=$1 and executor_id=$2`, id, executorID), "status")
}
