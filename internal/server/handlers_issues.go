package server

import (
	"net/http"
)

func (a *App) createIssue(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	var in struct{ Title, Description, Status, Priority, Assignee, Origin, ParentID string }
	if !readJSON(w, r, &in) {
		return
	}
	if in.Status == "" {
		in.Status = "backlog"
	}
	if in.Priority == "" {
		in.Priority = "none"
	}
	if in.Origin == "" {
		in.Origin = r.Header.Get("X-Shu-Origin")
	}
	assignee := struct{ typ, id string }{}
	if in.Assignee != "" {
		assignee, err = a.resolveAssigneeForResource(r.Context(), ws, in.Assignee, "")
		if err != nil {
			writeError(w, r, 400, err.Error())
			return
		}
		in.Status = "todo"
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	defer tx.Rollback(r.Context())
	var issueID string
	err = tx.QueryRow(r.Context(), `insert into issues(workspace_id,title,description,status,priority,assignee_type,assignee_id,origin,parent_issue_id) values($1,$2,$3,$4,$5,$6,$7,$8,$9) returning id::text`, ws, in.Title, in.Description, in.Status, in.Priority, assignee.typ, nullUUID(assignee.id), in.Origin, nullUUID(in.ParentID)).Scan(&issueID)
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"id": issueID, "title": in.Title, "status": in.Status})
}

func (a *App) listIssues(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	rows, err := a.db.Query(r.Context(), `select id::text,title,status,priority,assignee_type,coalesce(assignee_id::text,''),created_at,updated_at from issues where workspace_id=$1 order by updated_at desc limit 200`, ws)
	writeRows(w, rows, err, "id", "title", "status", "priority", "assignee_type", "assignee_id", "created_at", "updated_at")
}

func (a *App) getIssue(w http.ResponseWriter, r *http.Request) {
	if ws, err := a.issueWorkspace(r.Context(), r.PathValue("id")); err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `select id::text,title,description,status,priority,assignee_type,coalesce(assignee_id::text,''),created_at,updated_at from issues where id=$1`, r.PathValue("id")), "id", "title", "description", "status", "priority", "assignee_type", "assignee_id", "created_at", "updated_at")
}

func (a *App) updateIssue(w http.ResponseWriter, r *http.Request) {
	if ws, err := a.issueWorkspace(r.Context(), r.PathValue("id")); err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	var in struct{ Title, Description, Status, Priority string }
	if !readJSON(w, r, &in) {
		return
	}
	row := a.db.QueryRow(r.Context(), `update issues set title=coalesce(nullif($2,''),title), description=coalesce(nullif($3,''),description), status=coalesce(nullif($4,''),status), priority=coalesce(nullif($5,''),priority), updated_at=now() where id=$1 returning id::text,title,status,priority,updated_at`, r.PathValue("id"), in.Title, in.Description, in.Status, in.Priority)
	writeRow(w, row, "id", "title", "status", "priority", "updated_at")
}

func (a *App) createComment(w http.ResponseWriter, r *http.Request) {
	if ws, err := a.issueWorkspace(r.Context(), r.PathValue("id")); err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	var in struct{ Body string }
	if !readJSON(w, r, &in) {
		return
	}
	uid := currentUserID(r)
	var ws string
	if err := a.db.QueryRow(r.Context(), `select workspace_id::text from issues where id=$1`, r.PathValue("id")).Scan(&ws); err != nil {
		writeError(w, r, 404, err.Error())
		return
	}
	row := a.db.QueryRow(r.Context(), `insert into comments(workspace_id,issue_id,author_user_id,body) values($1,$2,$3,$4) returning id::text,body,created_at`, ws, r.PathValue("id"), nullUUID(uid), in.Body)
	writeRow(w, row, "id", "body", "created_at")
}

func (a *App) listComments(w http.ResponseWriter, r *http.Request) {
	if ws, err := a.issueWorkspace(r.Context(), r.PathValue("id")); err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	rows, err := a.db.Query(r.Context(), `select c.id::text,coalesce(u.name,''),c.body,c.resolved,c.created_at,c.updated_at from comments c left join users u on u.id=c.author_user_id where c.issue_id=$1 order by c.created_at`, r.PathValue("id"))
	writeRows(w, rows, err, "id", "author", "body", "resolved", "created_at", "updated_at")
}

func (a *App) issueTimeline(w http.ResponseWriter, r *http.Request) {
	if ws, err := a.issueWorkspace(r.Context(), r.PathValue("id")); err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	rows, err := a.db.Query(r.Context(), `
select 'issue' as type, id::text as id, title as text, created_at from issues where id=$1
union all
select 'comment', id::text, body, created_at from comments where issue_id=$1
union all
select 'attachment', id::text, file_name, created_at from attachments where issue_id=$1
order by created_at`, r.PathValue("id"))
	writeRows(w, rows, err, "type", "id", "text", "created_at")
}
