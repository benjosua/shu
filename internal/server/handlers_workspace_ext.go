package server

import (
	"net/http"
)

func (a *App) getWorkspace(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ws, err := a.workspaceID(r.Context(), id)
	if err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `select id::text,slug,name,description,issue_prefix,created_at from workspaces where id=$1`, ws), "id", "slug", "name", "description", "issue_prefix", "created_at")
}

func (a *App) updateWorkspace(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ws, err := a.workspaceID(r.Context(), id)
	if err != nil || a.requireWorkspaceRole(r, ws, RoleAdmin) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	var in struct{ Name, Description, IssuePrefix string }
	if !readJSON(w, r, &in) {
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `update workspaces set name=coalesce(nullif($2,''),name), description=coalesce(nullif($3,''),description), issue_prefix=coalesce(nullif($4,''),issue_prefix) where id=$1 returning id::text,slug,name,description,issue_prefix`, ws, in.Name, in.Description, in.IssuePrefix), "id", "slug", "name", "description", "issue_prefix")
}

func (a *App) deleteWorkspace(w http.ResponseWriter, r *http.Request) {
	ws, err := a.workspaceID(r.Context(), r.PathValue("id"))
	if err != nil || a.requireWorkspaceRole(r, ws, RoleOwner) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	_, err = a.db.Exec(r.Context(), `delete from workspaces where id=$1`, ws)
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"deleted": true})
}
