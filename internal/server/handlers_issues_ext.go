package server

import (
	"net/http"
	"strings"
)

func (a *App) searchIssues(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		q = strings.TrimSpace(r.URL.Query().Get("query"))
	}
	rows, err := a.db.Query(r.Context(), `select id::text,title,status,priority,created_at from issues where workspace_id=$1 and ($2='' or title ilike '%'||$2||'%' or description ilike '%'||$2||'%') order by updated_at desc limit 50`, ws, q)
	writeRows(w, rows, err, "id", "title", "status", "priority", "created_at")
}

func (a *App) groupedIssues(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	rows, err := a.db.Query(r.Context(), `select status, json_agg(json_build_object('id',id::text,'title',title,'priority',priority,'assignee_type',assignee_type,'assignee_id',assignee_id::text) order by updated_at desc) from issues where workspace_id=$1 group by status order by status`, ws)
	writeRows(w, rows, err, "status", "issues")
}

func (a *App) quickCreateIssue(w http.ResponseWriter, r *http.Request) {
	r.Header.Set("X-Shu-Origin", "quick_create")
	a.createIssue(w, r)
}

func (a *App) deleteIssue(w http.ResponseWriter, r *http.Request) {
	ws, err := a.issueWorkspace(r.Context(), r.PathValue("id"))
	if err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	_, err = a.db.Exec(r.Context(), `delete from issues where id=$1`, r.PathValue("id"))
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"deleted": true})
}

func (a *App) listChildIssues(w http.ResponseWriter, r *http.Request) {
	ws, err := a.issueWorkspace(r.Context(), r.PathValue("id"))
	if err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	rows, err := a.db.Query(r.Context(), `select id::text,title,status,priority,created_at from issues where parent_issue_id=$1 order by created_at`, r.PathValue("id"))
	writeRows(w, rows, err, "id", "title", "status", "priority", "created_at")
}

func (a *App) childIssueProgress(w http.ResponseWriter, r *http.Request) {
	ws, err := a.issueWorkspace(r.Context(), r.URL.Query().Get("issue_id"))
	if err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `select count(*)::int, count(*) filter (where status in ('done','completed'))::int from issues where parent_issue_id=$1`, r.URL.Query().Get("issue_id")), "total", "completed")
}

func (a *App) batchUpdateIssues(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	var in struct {
		IDs              []string
		Status, Priority string
	}
	if !readJSON(w, r, &in) {
		return
	}
	ct, err := a.db.Exec(r.Context(), `update issues set status=coalesce(nullif($3,''),status), priority=coalesce(nullif($4,''),priority), updated_at=now() where workspace_id=$1 and id::text=any($2)`, ws, in.IDs, in.Status, in.Priority)
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"updated": ct.RowsAffected()})
}

func (a *App) batchDeleteIssues(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	var in struct{ IDs []string }
	if !readJSON(w, r, &in) {
		return
	}
	ct, err := a.db.Exec(r.Context(), `delete from issues where workspace_id=$1 and id::text=any($2)`, ws, in.IDs)
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"deleted": ct.RowsAffected()})
}

func (a *App) assigneeFrequency(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	rows, err := a.db.Query(r.Context(), `select assignee_type,assignee_id::text,count(*)::int from issues where workspace_id=$1 and assignee_id is not null group by 1,2 order by 3 desc limit 50`, ws)
	writeRows(w, rows, err, "assignee_type", "assignee_id", "count")
}
