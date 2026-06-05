package server

import (
	"net/http"
)

func (a *App) getAutopilot(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "autopilots", r.PathValue("id"), RoleMember); !ok {
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `select a.id::text,a.name,a.prompt,a.assignee_type,a.assignee_id::text,a.enabled,min(t.next_run_at) filter (where t.enabled and t.kind='interval'),a.created_at from autopilots a left join autopilot_triggers t on t.autopilot_id=a.id where a.id=$1 group by a.id,a.name,a.prompt,a.assignee_type,a.assignee_id,a.enabled,a.created_at`, r.PathValue("id")), "id", "name", "prompt", "assignee_type", "assignee_id", "enabled", "next_run_at", "created_at")
}

func (a *App) updateAutopilot(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "autopilots", r.PathValue("id"), RoleAdmin); !ok {
		return
	}
	var in struct {
		Name, Prompt string
		Enabled      *bool
	}
	if !readJSON(w, r, &in) {
		return
	}
	enabled := any(nil)
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	writeRow(w, a.db.QueryRow(r.Context(), `update autopilots set name=coalesce(nullif($2,''),name), prompt=coalesce(nullif($3,''),prompt), enabled=coalesce($4,enabled) where id=$1 returning id::text,name,enabled`, r.PathValue("id"), in.Name, in.Prompt, enabled), "id", "name", "enabled")
}

func (a *App) deleteAutopilot(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "autopilots", r.PathValue("id"), RoleAdmin); !ok {
		return
	}
	_, err := a.db.Exec(r.Context(), `delete from autopilots where id=$1`, r.PathValue("id"))
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"deleted": true})
}

func (a *App) listAutopilotRuns(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "autopilots", r.PathValue("id"), RoleMember); !ok {
		return
	}
	rows, err := a.db.Query(r.Context(), `select id::text,status,coalesce(work_id::text,''),trigger_payload,started_at,completed_at from autopilot_runs where autopilot_id=$1 order by started_at desc limit 100`, r.PathValue("id"))
	writeRows(w, rows, err, "id", "status", "work_id", "trigger_payload", "started_at", "completed_at")
}

func (a *App) createAutopilotTrigger(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "autopilots", r.PathValue("id"), RoleAdmin); !ok {
		return
	}
	var in struct {
		Kind            string
		IntervalSeconds int
		Payload         map[string]any
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Kind == "" {
		in.Kind = "interval"
	}
	if in.Kind == "interval" && in.IntervalSeconds <= 0 {
		writeError(w, r, 400, "interval_seconds required for interval trigger")
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `insert into autopilot_triggers(autopilot_id,kind,interval_seconds,next_run_at,payload) values($1,$2,$3,now()+make_interval(secs => $3::int),$4) returning id::text,kind,interval_seconds,next_run_at`, r.PathValue("id"), in.Kind, in.IntervalSeconds, mustJSON(in.Payload)), "id", "kind", "interval_seconds", "next_run_at")
}

func (a *App) updateAutopilotTrigger(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "autopilots", r.PathValue("id"), RoleAdmin); !ok {
		return
	}
	var in struct {
		Enabled         *bool
		IntervalSeconds int
	}
	if !readJSON(w, r, &in) {
		return
	}
	enabled := any(nil)
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	writeRow(w, a.db.QueryRow(r.Context(), `update autopilot_triggers set enabled=coalesce($3,enabled), interval_seconds=case when $4=0 then interval_seconds else $4 end, next_run_at=case when $4=0 then next_run_at else now()+make_interval(secs => $4::int) end, updated_at=now() where autopilot_id=$1 and id=$2 returning id::text,enabled,interval_seconds,next_run_at`, r.PathValue("id"), r.PathValue("triggerId"), enabled, in.IntervalSeconds), "id", "enabled", "interval_seconds", "next_run_at")
}

func (a *App) deleteAutopilotTrigger(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "autopilots", r.PathValue("id"), RoleAdmin); !ok {
		return
	}
	_, err := a.db.Exec(r.Context(), `delete from autopilot_triggers where autopilot_id=$1 and id=$2`, r.PathValue("id"), r.PathValue("triggerId"))
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"deleted": true})
}
