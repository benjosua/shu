package server

import (
	"context"
	"net/http"
	"time"
)

func (a *App) createAutopilot(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	var in struct {
		Name, Prompt, Assignee string
		IntervalSeconds        int
	}
	if !readJSON(w, r, &in) {
		return
	}
	ass, err := a.resolveAssignee(r.Context(), ws, in.Assignee)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	defer tx.Rollback(r.Context())
	var id, name string
	if err := tx.QueryRow(r.Context(), `insert into autopilots(workspace_id,name,prompt,assignee_type,assignee_id) values($1,$2,$3,$4,$5) returning id::text,name`, ws, in.Name, in.Prompt, ass.Type, ass.ID).Scan(&id, &name); err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	if in.IntervalSeconds > 0 {
		if _, err := tx.Exec(r.Context(), `insert into autopilot_triggers(autopilot_id,kind,interval_seconds,next_run_at,payload) values($1,'interval',$2,now()+make_interval(secs => $2::int),$3)`, id, in.IntervalSeconds, mustJSON(map[string]any{"source": "schedule"})); err != nil {
			writeError(w, r, 500, err.Error())
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"id": id, "name": name})
}

func (a *App) listAutopilots(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	rows, err := a.db.Query(r.Context(), `select a.id::text,a.name,a.enabled,min(t.next_run_at) filter (where t.enabled and t.kind='interval') from autopilots a left join autopilot_triggers t on t.autopilot_id=a.id where a.workspace_id=$1 group by a.id,a.name,a.enabled,a.created_at order by a.created_at desc`, ws)
	writeRows(w, rows, err, "id", "name", "enabled", "next_run_at")
}

func (a *App) runAutopilotNow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := a.requireObjectRole(w, r, "autopilots", id, RoleAdmin); !ok {
		return
	}
	workID, err := a.enqueueAutopilot(r.Context(), id, nil)
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]string{"work_id": workID})
}

func (a *App) enqueueAutopilot(ctx context.Context, id string, payload map[string]any) (string, error) {
	var ws, prompt, atype, aid string
	err := a.db.QueryRow(ctx, `select workspace_id::text,prompt,assignee_type,assignee_id::text from autopilots where id=$1`, id).Scan(&ws, &prompt, &atype, &aid)
	if err != nil {
		return "", err
	}
	agentID, provider, prompt, err := a.workProfileForAssignee(ctx, ref(atype, aid), prompt)
	if err != nil {
		return "", err
	}
	executorID, err := a.pickExecutor(ctx, ws, provider, "")
	if err != nil {
		return "", err
	}
	var workID string
	err = a.db.QueryRow(ctx, `insert into work_items(workspace_id,kind,title,prompt,provider,agent_id,executor_id)
values($1,'autopilot',$2,$3,$4,$5,$6) returning id::text`, ws, "Autopilot: "+id, prompt, provider, nullUUID(agentID), executorID).Scan(&workID)
	if err != nil {
		return "", err
	}
	runID := ""
	if rid, err := a.runStore().Create(ctx, ws, "agent.autopilot", WorkQueued, ref(EntityWork, workID), map[string]any{"autopilot_id": id, "payload": payload}); err == nil {
		runID = rid
		_, _ = a.db.Exec(ctx, `update work_items set run_id=$2 where id=$1`, workID, rid)
	}
	if _, err := a.db.Exec(ctx, `insert into autopilot_runs(autopilot_id,workspace_id,status,work_id,run_id,trigger_payload) values($1,$2,$3,$4,$5,$6)`, id, ws, WorkQueued, workID, nullUUID(runID), mustJSON(payload)); err != nil {
		return "", err
	}
	a.publish(ctx, Event{Type: "work.created", WorkspaceID: ws, ExecutorID: executorID, Payload: map[string]string{"work_id": workID}, TS: time.Now()})
	return workID, nil
}
