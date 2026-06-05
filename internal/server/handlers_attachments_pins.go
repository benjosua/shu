package server

import (
	"net/http"
	"os"
)

func (a *App) listIssueAttachments(w http.ResponseWriter, r *http.Request) {
	if ws, err := a.issueWorkspace(r.Context(), r.PathValue("id")); err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	rows, err := a.db.Query(r.Context(), `select id::text,file_name,content_type,size_bytes,created_at from attachments where issue_id=$1 order by created_at`, r.PathValue("id"))
	writeRows(w, rows, err, "id", "file_name", "content_type", "size_bytes", "created_at")
}

func (a *App) deleteAttachment(w http.ResponseWriter, r *http.Request) {
	if ws, err := a.attachmentWorkspace(r.Context(), r.PathValue("id")); err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	var path string
	if err := a.db.QueryRow(r.Context(), `select storage_path from attachments where id=$1`, r.PathValue("id")).Scan(&path); err != nil {
		writeError(w, r, 404, err.Error())
		return
	}
	_, err := a.db.Exec(r.Context(), `delete from attachments where id=$1`, r.PathValue("id"))
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	_ = os.Remove(path)
	writeJSON(w, map[string]any{"deleted": true})
}
