package server

import (
	"net/http"
)

func (a *App) getSquad(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "squads", r.PathValue("id"), RoleMember); !ok {
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `select s.id::text,s.name,s.leader_agent_id::text,s.avatar_url,s.instructions,s.archived,s.created_at from squads s where s.id=$1`, r.PathValue("id")), "id", "name", "leader_agent_id", "avatar_url", "instructions", "archived", "created_at")
}

func (a *App) updateSquad(w http.ResponseWriter, r *http.Request) {
	ws, ok := a.requireObjectRole(w, r, "squads", r.PathValue("id"), RoleAdmin)
	if !ok {
		return
	}
	var in struct {
		Name, Leader, AvatarURL, Instructions string
		Archived                              *bool
	}
	if !readJSON(w, r, &in) {
		return
	}
	leader := ""
	if in.Leader != "" {
		if err := a.db.QueryRow(r.Context(), `select id::text from agents where workspace_id=$1 and (id::text=$2 or name=$2) limit 1`, ws, in.Leader).Scan(&leader); err != nil {
			writeError(w, r, 400, "leader not found")
			return
		}
	}
	archived := any(nil)
	if in.Archived != nil {
		archived = *in.Archived
	}
	writeRow(w, a.db.QueryRow(r.Context(), `update squads set name=coalesce(nullif($2,''),name), leader_agent_id=coalesce($3,leader_agent_id), avatar_url=coalesce(nullif($4,''),avatar_url), instructions=coalesce(nullif($5,''),instructions), archived=coalesce($6,archived) where id=$1 returning id::text,name,leader_agent_id::text,archived`, r.PathValue("id"), in.Name, nullUUID(leader), in.AvatarURL, in.Instructions, archived), "id", "name", "leader_agent_id", "archived")
}

func (a *App) deleteSquad(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "squads", r.PathValue("id"), RoleAdmin); !ok {
		return
	}
	_, err := a.db.Exec(r.Context(), `delete from squads where id=$1`, r.PathValue("id"))
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"deleted": true})
}

func (a *App) listSquadMembers(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "squads", r.PathValue("id"), RoleMember); !ok {
		return
	}
	rows, err := a.db.Query(r.Context(), `select member_type,member_id::text,role from squad_members where squad_id=$1 order by role,member_id`, r.PathValue("id"))
	writeRows(w, rows, err, "member_type", "member_id", "role")
}

func (a *App) removeSquadMember(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "squads", r.PathValue("id"), RoleAdmin); !ok {
		return
	}
	var in struct{ MemberType, MemberID string }
	if !readJSON(w, r, &in) {
		return
	}
	if in.MemberType == "" {
		in.MemberType = "agent"
	}
	_, err := a.db.Exec(r.Context(), `delete from squad_members where squad_id=$1 and member_type=$2 and member_id=$3`, r.PathValue("id"), in.MemberType, in.MemberID)
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"deleted": true})
}

func (a *App) updateSquadMemberRole(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "squads", r.PathValue("id"), RoleAdmin); !ok {
		return
	}
	var in struct{ MemberID, Role string }
	if !readJSON(w, r, &in) {
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `update squad_members set role=coalesce(nullif($3,''),role) where squad_id=$1 and member_id=$2 returning member_type,member_id::text,role`, r.PathValue("id"), in.MemberID, in.Role), "member_type", "member_id", "role")
}
