package server

import (
	"net/http"
)

func (a *App) listSkills(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	rows, err := a.db.Query(r.Context(), `select id::text,name,description,created_at,updated_at from skills where workspace_id=$1 order by name`, ws)
	writeRows(w, rows, err, "id", "name", "description", "created_at", "updated_at")
}

func (a *App) createSkill(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	var in struct{ Name, Description, Content string }
	if !readJSON(w, r, &in) {
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `insert into skills(workspace_id,name,description,content) values($1,$2,$3,$4) returning id::text,name,description`, ws, in.Name, in.Description, in.Content), "id", "name", "description")
}

func (a *App) getSkill(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "skills", r.PathValue("id"), RoleMember); !ok {
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `select id::text,name,description,content,created_at,updated_at from skills where id=$1`, r.PathValue("id")), "id", "name", "description", "content", "created_at", "updated_at")
}

func (a *App) updateSkill(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "skills", r.PathValue("id"), RoleMember); !ok {
		return
	}
	var in struct{ Name, Description, Content string }
	if !readJSON(w, r, &in) {
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `update skills set name=coalesce(nullif($2,''),name),description=coalesce(nullif($3,''),description),content=coalesce(nullif($4,''),content),updated_at=now() where id=$1 returning id::text,name,description,updated_at`, r.PathValue("id"), in.Name, in.Description, in.Content), "id", "name", "description", "updated_at")
}

func (a *App) deleteSkill(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "skills", r.PathValue("id"), RoleMember); !ok {
		return
	}
	_, err := a.db.Exec(r.Context(), `delete from skills where id=$1`, r.PathValue("id"))
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"deleted": true})
}

func (a *App) importSkill(w http.ResponseWriter, r *http.Request) { a.createSkill(w, r) }

func (a *App) listSkillFiles(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "skills", r.PathValue("id"), RoleMember); !ok {
		return
	}
	rows, err := a.db.Query(r.Context(), `select id::text,path,content,updated_at from skill_files where skill_id=$1 order by path`, r.PathValue("id"))
	writeRows(w, rows, err, "id", "path", "content", "updated_at")
}

func (a *App) upsertSkillFile(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "skills", r.PathValue("id"), RoleMember); !ok {
		return
	}
	var in struct{ Path, Content string }
	if !readJSON(w, r, &in) {
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `insert into skill_files(skill_id,path,content) values($1,$2,$3) on conflict(skill_id,path) do update set content=excluded.content,updated_at=now() returning id::text,path,updated_at`, r.PathValue("id"), in.Path, in.Content), "id", "path", "updated_at")
}

func (a *App) deleteSkillFile(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "skills", r.PathValue("id"), RoleMember); !ok {
		return
	}
	_, err := a.db.Exec(r.Context(), `delete from skill_files where skill_id=$1 and id=$2`, r.PathValue("id"), r.PathValue("fileId"))
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"deleted": true})
}
