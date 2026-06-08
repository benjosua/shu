package server

import (
	"context"
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
		Kind, Title, Prompt, Provider, Agent, AgentID, ResourceID, ParentID string
		AgentIDSnake                                                        string `json:"agent_id"`
		ResourceIDSnake                                                     string `json:"resource_id"`
		ParentIDSnake                                                       string `json:"parent_id"`
		Priority                                                            int
		Policy                                                              map[string]any
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Kind == "" {
		in.Kind = "work"
	}
	if in.ResourceID == "" {
		in.ResourceID = in.ResourceIDSnake
	}
	if in.ParentID == "" {
		in.ParentID = in.ParentIDSnake
	}
	agentID := in.AgentID
	if agentID == "" {
		agentID = in.AgentIDSnake
	}
	if agentID == "" {
		agentID = in.Agent
	}
	if agentID != "" {
		agentID, err = a.resolveAgentID(r.Context(), ws, agentID)
		if err != nil {
			writeError(w, r, 400, err.Error())
			return
		}
		if in.Provider == "" {
			in.Provider, err = a.agentProvider(r.Context(), agentID)
			if err != nil {
				writeError(w, r, 500, err.Error())
				return
			}
		}
	}
	work, err := a.workService().Enqueue(r.Context(), WorkSpec{
		WorkspaceID: ws,
		Kind:        in.Kind,
		Title:       in.Title,
		Prompt:      in.Prompt,
		Provider:    in.Provider,
		AgentID:     agentID,
		ResourceID:  in.ResourceID,
		ParentID:    in.ParentID,
		Priority:    in.Priority,
		Policy:      in.Policy,
		RunKind:     "agent.work",
		RunInput:    map[string]any{"title": in.Title, "kind": in.Kind},
	})
	if err != nil {
		writeError(w, r, 409, err.Error())
		return
	}
	a.publishWorkCreated(r.Context(), ws, work, nil)
	writeJSON(w, map[string]any{"id": work.WorkID, "kind": work.Kind, "title": work.Title, "status": work.Status, "executor_id": work.ExecutorID, "created_at": work.CreatedAt})
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
	writeRow(w, a.db.QueryRow(r.Context(), `select id::text,kind,coalesce(parent_id::text,''),title,prompt,provider,coalesce(agent_id::text,''),status,coalesce(resource_id::text,''),policy,coalesce(executor_id::text,''),result,error,created_at,started_at,completed_at from work_items where id=$1`, r.PathValue("id")), "id", "kind", "parent_id", "title", "prompt", "provider", "agent_id", "status", "resource_id", "policy", "executor_id", "result", "error", "created_at", "started_at", "completed_at")
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
  update work_items set status=$2, dispatched_at=now()
  where id=(
    select id from work_items where status=$3 and executor_id=$1 order by priority desc, created_at asc for update skip locked limit 1
  ) returning *
)
select c.id::text,c.workspace_id::text,c.title,c.prompt,c.provider,c.executor_id::text,e.mode,
       coalesce(r.kind,''),coalesce(r.locator,''),
       coalesce(c.agent_id::text,''),coalesce(a.name,c.provider),coalesce(a.provider,c.provider),
       coalesce(a.instructions,''),coalesce(a.model,''),coalesce(a.custom_env,'{}'::jsonb),coalesce(a.custom_args,'[]'::jsonb),
       c.policy
from claimed c
join executors e on e.id=c.executor_id
left join resources r on r.id=c.resource_id
left join agents a on a.id=c.agent_id`, executorID, WorkDispatched, WorkQueued)
	var id, workWS, title, prompt, provider, exID, mode, resourceKind, resourceLocator string
	var agentID, agentName, agentProvider, agentInstructions, agentModel string
	var agentEnv, agentArgs, policy []byte
	if err := row.Scan(&id, &workWS, &title, &prompt, &provider, &exID, &mode, &resourceKind, &resourceLocator, &agentID, &agentName, &agentProvider, &agentInstructions, &agentModel, &agentEnv, &agentArgs, &policy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, map[string]any{})
			return
		}
		writeError(w, r, 500, err.Error())
		return
	}
	if agentID != "" {
		agentInstructions = a.agentInstructionsWithSkills(r.Context(), agentID, agentInstructions)
	}
	var customEnv map[string]string
	var customArgs []string
	_ = json.Unmarshal(agentEnv, &customEnv)
	_ = json.Unmarshal(agentArgs, &customArgs)
	if customEnv == nil {
		customEnv = map[string]string{}
	}
	if customArgs == nil {
		customArgs = []string{}
	}
	writeJSON(w, map[string]any{
		"id": id, "workspace_id": workWS, "title": title, "body": prompt,
		"agent_id": agentID, "executor_id": exID, "executor_mode": mode,
		"resource": map[string]string{"kind": resourceKind, "locator": resourceLocator},
		"policy":   json.RawMessage(policy),
		"agent":    map[string]any{"id": agentID, "name": agentName, "provider": agentProvider, "instructions": agentInstructions, "model": agentModel, "custom_env": customEnv, "custom_args": customArgs},
	})
}

func (a *App) agentInstructionsWithSkills(ctx context.Context, agentID, base string) string {
	rows, err := a.db.Query(ctx, `select s.id::text,s.name,s.description,s.content from agent_skills ag join skills s on s.id=ag.skill_id where ag.agent_id=$1 order by s.name`, agentID)
	if err != nil {
		return base
	}
	defer rows.Close()
	var b strings.Builder
	b.WriteString(strings.TrimSpace(base))
	for rows.Next() {
		var id, name, desc, content string
		if err := rows.Scan(&id, &name, &desc, &content); err != nil {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("## Skill: ")
		b.WriteString(name)
		b.WriteString("\n")
		if strings.TrimSpace(desc) != "" {
			b.WriteString(strings.TrimSpace(desc))
			b.WriteString("\n")
		}
		if strings.TrimSpace(content) != "" {
			b.WriteString(strings.TrimSpace(content))
			b.WriteString("\n")
		}
		fileRows, err := a.db.Query(ctx, `select path,content from skill_files where skill_id=$1 order by path`, id)
		if err != nil {
			continue
		}
		for fileRows.Next() {
			var path, fileContent string
			if err := fileRows.Scan(&path, &fileContent); err == nil && strings.TrimSpace(fileContent) != "" {
				b.WriteString("\n### ")
				b.WriteString(path)
				b.WriteString("\n")
				b.WriteString(strings.TrimSpace(fileContent))
				b.WriteString("\n")
			}
		}
		fileRows.Close()
	}
	return b.String()
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
	var runID string
	if err := a.db.QueryRow(r.Context(), `update work_items set status=$3, started_at=now() where id=$1 and executor_id=$2 and status=$4 returning workspace_id::text,coalesce(run_id::text,'')`, id, in.ExecutorID, WorkRunning, WorkDispatched).Scan(&ws, &runID); err != nil {
		writeError(w, r, 409, err.Error())
		return
	}
	a.runStore().Start(r.Context(), runID)
	a.publish(r.Context(), Event{Type: "work.running", WorkspaceID: ws, Payload: map[string]string{"work_id": id}, TS: time.Now()})
	writeJSON(w, map[string]string{"status": WorkRunning})
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
	a.postProcessExternalArtifact(r.Context(), ws, id, in.Type, in.Data)
	a.activityStore().Record(r.Context(), ws, ref(EntityWork, id), "artifact.created", EntityRef{}, map[string]any{"type": in.Type})
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
	dbStatus := WorkCompleted
	if status == "fail" {
		dbStatus = WorkFailed
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
	var runID string
	if err := a.db.QueryRow(r.Context(), `update work_items set status=$2,result=$3,error=$4,completed_at=now() where id=$1 and executor_id=$5 and status in ($6,$7) returning workspace_id::text,coalesce(run_id::text,'')`, id, dbStatus, in.Result, in.Error, in.ExecutorID, WorkDispatched, WorkRunning).Scan(&ws, &runID); err != nil {
		writeError(w, r, 409, err.Error())
		return
	}
	if dbStatus == WorkCompleted {
		_, _ = a.db.Exec(r.Context(), `update issues set status=$2, updated_at=now() where id=(select (policy->>'issue_id')::uuid from work_items where id=$1 and kind='issue' and policy ? 'issue_id')`, id, IssueDone)
	} else {
		_, _ = a.db.Exec(r.Context(), `update issues set status=$2, updated_at=now() where id=(select (policy->>'issue_id')::uuid from work_items where id=$1 and kind='issue' and policy ? 'issue_id')`, id, IssueTodo)
	}
	a.runStore().Finish(r.Context(), runID, dbStatus, map[string]any{"result": in.Result}, in.Error)
	_, _ = a.db.Exec(r.Context(), `update autopilot_runs set status=$2, completed_at=now() where work_id=$1`, id, dbStatus)
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

func (a *App) cancelWork(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ws, err := a.workWorkspace(r.Context(), id)
	if err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	var runID string
	if err := a.db.QueryRow(r.Context(), `update work_items set status=$2,error=case when error='' then 'cancelled by user' else error end,completed_at=now() where id=$1 and status in ($3,$4,$5) returning coalesce(run_id::text,'')`, id, WorkCancelled, WorkQueued, WorkDispatched, WorkRunning).Scan(&runID); err != nil {
		writeError(w, r, 409, err.Error())
		return
	}
	a.runStore().Finish(r.Context(), runID, WorkCancelled, nil, "cancelled by user")
	_, _ = a.db.Exec(r.Context(), `update autopilot_runs set status=$2, completed_at=now() where work_id=$1`, id, WorkCancelled)
	_, _ = a.db.Exec(r.Context(), `update issues set status=$2, updated_at=now() where id=(select (policy->>'issue_id')::uuid from work_items where id=$1 and kind='issue' and policy ? 'issue_id')`, id, IssueTodo)
	a.publish(r.Context(), Event{Type: "work.cancelled", WorkspaceID: ws, Payload: map[string]string{"work_id": id}, TS: time.Now()})
	writeJSON(w, map[string]string{"status": WorkCancelled})
}
