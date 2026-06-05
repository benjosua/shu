package server

import (
	"net/http"
)

func (a *App) getMe(w http.ResponseWriter, r *http.Request) {
	uid := currentUserID(r)
	writeRow(w, a.db.QueryRow(r.Context(), `select id::text,name,email,avatar_url,language,onboarding,created_at from users where id=$1`, uid), "id", "name", "email", "avatar_url", "language", "onboarding", "created_at")
}

func (a *App) updateMe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name, Email, AvatarURL, Language string
		Onboarding                       map[string]any
	}
	if !readJSON(w, r, &in) {
		return
	}
	uid := currentUserID(r)
	row := a.db.QueryRow(r.Context(), `update users set name=coalesce(nullif($2,''),name), email=coalesce(nullif($3,''),email), avatar_url=coalesce(nullif($4,''),avatar_url), language=coalesce(nullif($5,''),language), onboarding=case when $6::jsonb='{}'::jsonb then onboarding else $6::jsonb end where id=$1 returning id::text,name,email,avatar_url,language,onboarding`, uid, in.Name, in.Email, in.AvatarURL, in.Language, mustJSON(in.Onboarding))
	writeRow(w, row, "id", "name", "email", "avatar_url", "language", "onboarding")
}

func (a *App) getConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"name": "shu", "api_first": true, "executor_modes": []string{"local", "cloud"}})
}
