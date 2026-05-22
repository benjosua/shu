package server

import (
	"net/http"
	"time"
)

func (a *App) listPersonalAccessTokens(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	if uid == "" {
		writeError(w, r, 401, "user token required")
		return
	}
	rows, err := a.db.Query(r.Context(), `select id::text,name,expires_at,last_used_at,created_at from personal_access_tokens where user_id=$1 order by created_at desc`, uid)
	writeRows(w, rows, err, "id", "name", "expires_at", "last_used_at", "created_at")
}

func (a *App) createPersonalAccessToken(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	if uid == "" {
		writeError(w, r, 401, "user token required")
		return
	}
	var in struct {
		Name      string
		ExpiresIn int64
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Name == "" {
		in.Name = "cli"
	}
	tok, hash := newToken()
	var expires any
	if in.ExpiresIn > 0 {
		expires = time.Now().Add(time.Duration(in.ExpiresIn) * time.Second)
	}
	row := a.db.QueryRow(r.Context(), `insert into personal_access_tokens(user_id,name,token_hash,expires_at) values($1,$2,$3,$4) returning id::text,name,created_at`, uid, in.Name, hash, expires)
	vals := map[string]any{"token": tok}
	var id, name string
	var created any
	if err := row.Scan(&id, &name, &created); err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	vals["id"] = id
	vals["name"] = name
	vals["created_at"] = created
	writeJSON(w, vals)
}

func (a *App) revokePersonalAccessToken(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	if uid == "" {
		writeError(w, r, 401, "user token required")
		return
	}
	_, err := a.db.Exec(r.Context(), `delete from personal_access_tokens where id=$1 and user_id=$2`, r.PathValue("id"), uid)
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "revoked"})
}
