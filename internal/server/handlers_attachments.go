package server

import (
	"encoding/base64"
	"github.com/google/uuid"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) createAttachment(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		a.createMultipartAttachment(w, r)
		return
	}
	var in struct{ IssueID, CommentID, FileName, ContentType, ContentBase64 string }
	if !readJSON(w, r, &in) {
		return
	}
	if in.FileName == "" {
		writeError(w, r, 400, "fileName required")
		return
	}
	data, err := base64.StdEncoding.DecodeString(in.ContentBase64)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	if in.ContentType == "" {
		in.ContentType = "application/octet-stream"
	}
	a.persistAttachment(w, r, in.IssueID, in.CommentID, in.FileName, in.ContentType, data)
}

func (a *App) createMultipartAttachment(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 128<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeError(w, r, 400, "file field required")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	ct := hdr.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	a.persistAttachment(w, r, r.FormValue("issue_id"), r.FormValue("comment_id"), hdr.Filename, ct, data)
}

func (a *App) persistAttachment(w http.ResponseWriter, r *http.Request, issueID, commentID, fileName, contentType string, data []byte) {
	ws := ""
	if issueID != "" {
		_ = a.db.QueryRow(r.Context(), `select workspace_id::text from issues where id=$1`, issueID).Scan(&ws)
	}
	if commentID != "" {
		var cws string
		_ = a.db.QueryRow(r.Context(), `select workspace_id::text from comments where id=$1`, commentID).Scan(&cws)
		if ws == "" {
			ws = cws
		} else if cws != "" && cws != ws {
			writeError(w, r, 400, "issue/comment workspace mismatch")
			return
		}
	}
	if ws == "" {
		writeError(w, r, 400, "issue_id/issueId or comment_id/commentId required")
		return
	}
	if err := a.requireWorkspaceRole(r, ws, RoleMember); err != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	id := uuid.NewString()
	path := attachmentPath(a.cfg.UploadRoot, ws, id, fileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	row := a.db.QueryRow(r.Context(), `insert into attachments(id,workspace_id,issue_id,comment_id,file_name,content_type,size_bytes,storage_path) values($1,$2,$3,$4,$5,$6,$7,$8) returning id::text,file_name,content_type,size_bytes`, id, ws, nullUUID(issueID), nullUUID(commentID), fileName, contentType, len(data), path)
	writeRow(w, row, "id", "file_name", "content_type", "size_bytes")
}

func (a *App) getAttachment(w http.ResponseWriter, r *http.Request) {
	if ws, err := a.attachmentWorkspace(r.Context(), r.PathValue("id")); err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `select id::text,file_name,content_type,size_bytes,created_at from attachments where id=$1`, r.PathValue("id")), "id", "file_name", "content_type", "size_bytes", "created_at")
}

func (a *App) getAttachmentContent(w http.ResponseWriter, r *http.Request) {
	if ws, err := a.attachmentWorkspace(r.Context(), r.PathValue("id")); err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	var name, ct, path string
	if err := a.db.QueryRow(r.Context(), `select file_name,content_type,storage_path from attachments where id=$1`, r.PathValue("id")).Scan(&name, &ct, &path); err != nil {
		writeError(w, r, 404, err.Error())
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(name, `"`, "")+`"`)
	http.ServeFile(w, r, path)
}
