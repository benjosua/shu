package server

import (
	"net/http"
)

func (a *App) createSquad(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	var in struct{ Name, Leader string }
	if !readJSON(w, r, &in) {
		return
	}
	var leader string
	if err := a.db.QueryRow(r.Context(), `select id::text from agents where workspace_id=$1 and (name=$2 or id::text=$2)`, ws, in.Leader).Scan(&leader); err != nil {
		writeError(w, r, 400, "leader not found")
		return
	}
	row := a.db.QueryRow(r.Context(), `insert into squads(workspace_id,name,leader_agent_id) values($1,$2,$3) returning id::text,name`, ws, in.Name, leader)
	writeRow(w, row, "id", "name")
}

func (a *App) listSquads(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	rows, err := a.db.Query(r.Context(), `select s.id::text,s.name,a.name from squads s join agents a on a.id=s.leader_agent_id where s.workspace_id=$1 order by s.name`, ws)
	writeRows(w, rows, err, "id", "name", "leader")
}

func (a *App) addSquadMember(w http.ResponseWriter, r *http.Request) {
	ws, ok := a.requireObjectRole(w, r, "squads", r.PathValue("id"), RoleAdmin)
	if !ok {
		return
	}
	var in struct{ Agent string }
	if !readJSON(w, r, &in) {
		return
	}
	var aid string
	if err := a.db.QueryRow(r.Context(), `select id::text from agents where workspace_id=$1 and (name=$2 or id::text=$2) limit 1`, ws, in.Agent).Scan(&aid); err != nil {
		writeError(w, r, 400, "agent not found")
		return
	}
	_, err := a.db.Exec(r.Context(), `insert into squad_members(squad_id,member_type,member_id) values($1,'agent',$2) on conflict do nothing`, r.PathValue("id"), aid)
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]string{"ok": "true"})
}
