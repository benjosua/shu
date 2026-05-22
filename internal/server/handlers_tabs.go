package server

import (
	"net/http"
)

func (a *App) openTab(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	var in struct{ Route, Title string }
	if !readJSON(w, r, &in) {
		return
	}
	if in.Title == "" {
		in.Title = in.Route
	}
	if _, err := a.db.Exec(r.Context(), `update workspace_tabs set active=false where workspace_id=$1`, ws); err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	row := a.db.QueryRow(r.Context(), `insert into workspace_tabs(workspace_id,title,route,active) values($1,$2,$3,true) returning id::text,title,route,active`, ws, in.Title, in.Route)
	writeRow(w, row, "id", "title", "route", "active")
}

func (a *App) listTabs(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	rows, err := a.db.Query(r.Context(), `select id::text,title,route,active,updated_at from workspace_tabs where workspace_id=$1 order by updated_at desc`, ws)
	writeRows(w, rows, err, "id", "title", "route", "active", "updated_at")
}
