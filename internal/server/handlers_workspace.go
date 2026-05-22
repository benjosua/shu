package server

import "net/http"

func (a *App) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var in struct{ Slug, Name string }
	if !readJSON(w, r, &in) {
		return
	}
	if in.Name == "" {
		in.Name = in.Slug
	}
	uid := currentUserID(r)
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	defer tx.Rollback(r.Context())
	var id, slug, name string
	if err := tx.QueryRow(r.Context(), `insert into workspaces(slug,name) values($1,$2) returning id::text, slug, name`, in.Slug, in.Name).Scan(&id, &slug, &name); err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	if uid != "" {
		_, _ = tx.Exec(r.Context(), `insert into workspace_members(workspace_id,user_id,role) values($1,$2,'owner') on conflict do nothing`, id, uid)
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]string{"id": id, "slug": slug, "name": name})
}
func (a *App) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	if uid != "" {
		rows, err := a.db.Query(r.Context(), `select w.id::text, w.slug, w.name, m.role, w.created_at from workspaces w join workspace_members m on m.workspace_id=w.id where m.user_id=$1 order by w.created_at`, uid)
		writeRows(w, rows, err, "id", "slug", "name", "role", "created_at")
		return
	}
	rows, err := a.db.Query(r.Context(), `select id::text, slug, name, created_at from workspaces order by created_at`)
	writeRows(w, rows, err, "id", "slug", "name", "created_at")
}

func (a *App) listWorkspaceMembers(w http.ResponseWriter, r *http.Request) {
	ws, err := a.workspaceID(r.Context(), r.PathValue("id"))
	if err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	rows, err := a.db.Query(r.Context(), `select m.id::text, coalesce(u.id::text,''), coalesce(u.name,''), m.role, m.created_at from workspace_members m left join users u on u.id=m.user_id where m.workspace_id=$1 order by m.created_at`, ws)
	writeRows(w, rows, err, "id", "user_id", "name", "role", "created_at")
}

func (a *App) addWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	ws, err := a.workspaceID(r.Context(), r.PathValue("id"))
	if err != nil || a.requireWorkspaceRole(r, ws, RoleAdmin) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	var in struct{ User, Role string }
	if !readJSON(w, r, &in) {
		return
	}
	if in.Role == "" {
		in.Role = "member"
	}
	var uid string
	err = a.db.QueryRow(r.Context(), `select id::text from users where id::text=$1 or name=$1`, in.User).Scan(&uid)
	if err != nil {
		writeError(w, r, 404, "user not found")
		return
	}
	row := a.db.QueryRow(r.Context(), `insert into workspace_members(workspace_id,user_id,role) values($1,$2,$3) on conflict(workspace_id,user_id) do update set role=excluded.role returning id::text,user_id::text,role`, ws, uid, in.Role)
	writeRow(w, row, "id", "user_id", "role")
}
