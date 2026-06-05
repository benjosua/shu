package server

import (
	"net/http"
)

func (a *App) createChatSession(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	var in struct{ Title, Agent string }
	if !readJSON(w, r, &in) {
		return
	}
	if in.Title == "" {
		in.Title = "New chat"
	}
	aid := ""
	if in.Agent != "" {
		aid, err = a.resolveAgentID(r.Context(), ws, in.Agent)
		if err != nil {
			writeError(w, r, 400, err.Error())
			return
		}
	}
	writeRow(w, a.db.QueryRow(r.Context(), `insert into chat_sessions(workspace_id,title,agent_id) values($1,$2,$3) returning id::text,title,coalesce(agent_id::text,''),created_at`, ws, in.Title, nullUUID(aid)), "id", "title", "agent_id", "created_at")
}

func (a *App) listChatSessions(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	rows, err := a.db.Query(r.Context(), `select id::text,title,coalesce(agent_id::text,''),created_at,updated_at from chat_sessions where workspace_id=$1 order by updated_at desc`, ws)
	writeRows(w, rows, err, "id", "title", "agent_id", "created_at", "updated_at")
}

func (a *App) getChatSession(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "chat_sessions", r.PathValue("sessionId"), RoleMember); !ok {
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `select id::text,title,coalesce(agent_id::text,''),created_at,updated_at from chat_sessions where id=$1`, r.PathValue("sessionId")), "id", "title", "agent_id", "created_at", "updated_at")
}

func (a *App) updateChatSession(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "chat_sessions", r.PathValue("sessionId"), RoleMember); !ok {
		return
	}
	var in struct{ Title string }
	if !readJSON(w, r, &in) {
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `update chat_sessions set title=coalesce(nullif($2,''),title),updated_at=now() where id=$1 returning id::text,title,updated_at`, r.PathValue("sessionId"), in.Title), "id", "title", "updated_at")
}

func (a *App) deleteChatSession(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "chat_sessions", r.PathValue("sessionId"), RoleMember); !ok {
		return
	}
	_, err := a.db.Exec(r.Context(), `delete from chat_sessions where id=$1`, r.PathValue("sessionId"))
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"deleted": true})
}

func (a *App) sendChatMessage(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "chat_sessions", r.PathValue("sessionId"), RoleMember); !ok {
		return
	}
	var in struct{ Role, Content string }
	if !readJSON(w, r, &in) {
		return
	}
	if in.Role == "" {
		in.Role = "user"
	}
	var ws string
	if err := a.db.QueryRow(r.Context(), `select workspace_id::text from chat_sessions where id=$1`, r.PathValue("sessionId")).Scan(&ws); err != nil {
		writeError(w, r, 404, err.Error())
		return
	}
	var id int64
	var role, content string
	var created any
	if err := a.db.QueryRow(r.Context(), `insert into chat_messages(workspace_id,session_id,role,content) values($1,$2,$3,$4) returning id,role,content,created_at`, ws, r.PathValue("sessionId"), in.Role, in.Content).Scan(&id, &role, &content, &created); err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	a.activityStore().Record(r.Context(), ws, ref(EntityChatSession, r.PathValue("sessionId")), "chat.message.created", EntityRef{}, map[string]any{"message_id": id, "role": role})
	writeJSON(w, map[string]any{"id": id, "role": role, "content": content, "created_at": created})
}

func (a *App) listChatMessages(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "chat_sessions", r.PathValue("sessionId"), RoleMember); !ok {
		return
	}
	rows, err := a.db.Query(r.Context(), `select id,role,content,created_at from chat_messages where session_id=$1 order by id`, r.PathValue("sessionId"))
	writeRows(w, rows, err, "id", "role", "content", "created_at")
}
