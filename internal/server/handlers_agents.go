package server

import (
	"context"
	"net/http"
)

func (a *App) getAgent(w http.ResponseWriter, r *http.Request) {
	ws, err := a.agentWorkspace(r.Context(), r.PathValue("id"))
	if err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `select id::text,name,provider,description,model,instructions,custom_env,custom_args,avatar_url,archived from agents where id=$1`, r.PathValue("id")), "id", "name", "provider", "description", "model", "instructions", "custom_env", "custom_args", "avatar_url", "archived")
}

func (a *App) updateAgent(w http.ResponseWriter, r *http.Request) {
	ws, err := a.agentWorkspace(r.Context(), r.PathValue("id"))
	if err != nil || a.requireWorkspaceRole(r, ws, RoleAdmin) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	var in struct {
		Name, Description, Instructions, Model, AvatarURL string
		CustomEnv                                         map[string]string
		CustomArgs                                        []string
	}
	if !readJSON(w, r, &in) {
		return
	}
	row := a.db.QueryRow(r.Context(), `update agents set name=coalesce(nullif($2,''),name), description=coalesce(nullif($3,''),description), instructions=coalesce(nullif($4,''),instructions), model=coalesce(nullif($5,''),model), avatar_url=coalesce(nullif($6,''),avatar_url), custom_env=case when $7::jsonb='null'::jsonb then custom_env else $7::jsonb end, custom_args=case when $8::jsonb='null'::jsonb then custom_args else $8::jsonb end where id=$1 returning id::text,name,provider`, r.PathValue("id"), in.Name, in.Description, in.Instructions, in.Model, in.AvatarURL, mustJSON(in.CustomEnv), mustJSON(in.CustomArgs))
	writeRow(w, row, "id", "name", "provider")
}

func (a *App) archiveAgent(w http.ResponseWriter, r *http.Request) { a.setAgentArchived(w, r, true) }

func (a *App) restoreAgent(w http.ResponseWriter, r *http.Request) { a.setAgentArchived(w, r, false) }

func (a *App) setAgentArchived(w http.ResponseWriter, r *http.Request, archived bool) {
	ws, err := a.agentWorkspace(r.Context(), r.PathValue("id"))
	if err != nil || a.requireWorkspaceRole(r, ws, RoleAdmin) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `update agents set archived=$2 where id=$1 returning id::text,name,archived`, r.PathValue("id"), archived), "id", "name", "archived")
}

func (a *App) agentWorkspace(ctx context.Context, id string) (string, error) {
	var ws string
	err := a.db.QueryRow(ctx, `select workspace_id::text from agents where id=$1`, id).Scan(&ws)
	return ws, err
}

func (a *App) listAgentSkills(w http.ResponseWriter, r *http.Request) {
	ws, err := a.agentWorkspace(r.Context(), r.PathValue("id"))
	if err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	rows, err := a.db.Query(r.Context(), `select s.id::text,s.name,s.description from agent_skills ag join skills s on s.id=ag.skill_id where ag.agent_id=$1 order by s.name`, r.PathValue("id"))
	writeRows(w, rows, err, "id", "name", "description")
}

func (a *App) setAgentSkills(w http.ResponseWriter, r *http.Request) {
	ws, err := a.agentWorkspace(r.Context(), r.PathValue("id"))
	if err != nil || a.requireWorkspaceRole(r, ws, RoleAdmin) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	var in struct{ SkillIDs []string }
	if !readJSON(w, r, &in) {
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `delete from agent_skills where agent_id=$1`, r.PathValue("id"))
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	for _, sid := range in.SkillIDs {
		var skillWS string
		if err = tx.QueryRow(r.Context(), `select workspace_id::text from skills where id=$1`, sid).Scan(&skillWS); err != nil || skillWS != ws {
			writeError(w, r, 403, "forbidden")
			return
		}
		_, err = tx.Exec(r.Context(), `insert into agent_skills(agent_id,skill_id) values($1,$2) on conflict do nothing`, r.PathValue("id"), sid)
		if err != nil {
			writeError(w, r, 500, err.Error())
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"count": len(in.SkillIDs)})
}
