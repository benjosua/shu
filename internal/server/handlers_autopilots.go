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
	row := a.db.QueryRow(r.Context(), `insert into autopilots(workspace_id,name,prompt,assignee_type,assignee_id,trigger_type,cron_interval_seconds,next_run_at) values($1,$2,$3,$4,$5,'interval',$6::int,now()+make_interval(secs => $6::int)) returning id::text,name`, ws, in.Name, in.Prompt, ass.typ, ass.id, in.IntervalSeconds)
	writeRow(w, row, "id", "name")
}

func (a *App) listAutopilots(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	rows, err := a.db.Query(r.Context(), `select id::text,name,enabled,next_run_at from autopilots where workspace_id=$1 order by created_at desc`, ws)
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
	provider := "codex"
	if atype == "agent" {
		_ = a.db.QueryRow(ctx, `select provider from agents where id=$1`, aid).Scan(&provider)
	}
	executorID, err := a.pickExecutor(ctx, ws, provider, "")
	if err != nil {
		return "", err
	}
	var workID string
	err = a.db.QueryRow(ctx, `insert into work_items(workspace_id,kind,title,prompt,provider,executor_id)
values($1,'autopilot',$2,$3,$4,$5) returning id::text`, ws, "Autopilot: "+id, prompt, provider, executorID).Scan(&workID)
	if err != nil {
		return "", err
	}
	if _, err := a.db.Exec(ctx, `insert into autopilot_runs(autopilot_id,workspace_id,status,work_id,trigger_payload) values($1,$2,'queued',$3,$4)`, id, ws, workID, mustJSON(payload)); err != nil {
		return "", err
	}
	a.publish(ctx, Event{Type: "work.created", WorkspaceID: ws, ExecutorID: executorID, Payload: map[string]string{"work_id": workID}, TS: time.Now()})
	return workID, nil
}

func (a *App) autopilotScheduler(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for range t.C {
		rows, err := a.db.Query(ctx, `select id::text, cron_interval_seconds from autopilots where enabled=true and trigger_type='interval' and next_run_at <= now()`)
		if err != nil {
			continue
		}
		for rows.Next() {
			var id string
			var sec int
			_ = rows.Scan(&id, &sec)
			_, _ = a.enqueueAutopilot(ctx, id, map[string]any{"source": "schedule"})
			_, _ = a.db.Exec(ctx, `update autopilots set next_run_at=now()+($2||' seconds')::interval where id=$1`, id, sec)
		}
		rows.Close()
	}
}
