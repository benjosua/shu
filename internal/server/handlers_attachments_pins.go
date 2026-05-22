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

func (a *App) listPins(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	rows, err := a.db.Query(r.Context(), `select id::text,item_type,item_id::text,sort_order,created_at from pinned_items where workspace_id=$1 order by sort_order,created_at`, ws)
	writeRows(w, rows, err, "id", "item_type", "item_id", "sort_order", "created_at")
}

func (a *App) createPin(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	var in struct {
		ItemType, ItemID string
		SortOrder        int
	}
	if !readJSON(w, r, &in) {
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `insert into pinned_items(workspace_id,item_type,item_id,sort_order) values($1,$2,$3,$4) on conflict(workspace_id,item_type,item_id) do update set sort_order=excluded.sort_order returning id::text,item_type,item_id::text,sort_order`, ws, in.ItemType, in.ItemID, in.SortOrder), "id", "item_type", "item_id", "sort_order")
}

func (a *App) deletePin(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	_, err = a.db.Exec(r.Context(), `delete from pinned_items where workspace_id=$1 and item_type=$2 and item_id=$3`, ws, r.PathValue("itemType"), r.PathValue("itemId"))
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"deleted": true})
}
