package server

import (
	"context"
	"net/http"
)

func (a *App) updateComment(w http.ResponseWriter, r *http.Request) {
	ws, err := a.commentWorkspace(r.Context(), r.PathValue("commentId"))
	if err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	var in struct{ Body string }
	if !readJSON(w, r, &in) {
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `update comments set body=coalesce(nullif($2,''),body),updated_at=now() where id=$1 returning id::text,body,updated_at`, r.PathValue("commentId"), in.Body), "id", "body", "updated_at")
}

func (a *App) deleteComment(w http.ResponseWriter, r *http.Request) {
	ws, err := a.commentWorkspace(r.Context(), r.PathValue("commentId"))
	if err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	_, err = a.db.Exec(r.Context(), `delete from comments where id=$1`, r.PathValue("commentId"))
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"deleted": true})
}

func (a *App) resolveComment(w http.ResponseWriter, r *http.Request) {
	a.setCommentResolved(w, r, true)
}

func (a *App) unresolveComment(w http.ResponseWriter, r *http.Request) {
	a.setCommentResolved(w, r, false)
}

func (a *App) setCommentResolved(w http.ResponseWriter, r *http.Request, resolved bool) {
	ws, err := a.commentWorkspace(r.Context(), r.PathValue("commentId"))
	if err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `update comments set resolved=$2,resolved_at=case when $2 then now() else null end,updated_at=now() where id=$1 returning id::text,resolved,resolved_at`, r.PathValue("commentId"), resolved), "id", "resolved", "resolved_at")
}

func (a *App) commentWorkspace(ctx context.Context, id string) (string, error) {
	var ws string
	err := a.db.QueryRow(ctx, `select workspace_id::text from comments where id=$1`, id).Scan(&ws)
	return ws, err
}

func (a *App) listLabels(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	rows, err := a.db.Query(r.Context(), `select id::text,name,color,description,created_at,updated_at from labels where workspace_id=$1 order by name`, ws)
	writeRows(w, rows, err, "id", "name", "color", "description", "created_at", "updated_at")
}

func (a *App) createLabel(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	var in struct{ Name, Color, Description string }
	if !readJSON(w, r, &in) {
		return
	}
	if in.Color == "" {
		in.Color = "#808080"
	}
	writeRow(w, a.db.QueryRow(r.Context(), `insert into labels(workspace_id,name,color,description) values($1,$2,$3,$4) returning id::text,name,color,description`, ws, in.Name, in.Color, in.Description), "id", "name", "color", "description")
}

func (a *App) getLabel(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "labels", r.PathValue("id"), RoleMember); !ok {
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `select id::text,name,color,description,created_at,updated_at from labels where id=$1`, r.PathValue("id")), "id", "name", "color", "description", "created_at", "updated_at")
}

func (a *App) updateLabel(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "labels", r.PathValue("id"), RoleMember); !ok {
		return
	}
	var in struct{ Name, Color, Description string }
	if !readJSON(w, r, &in) {
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `update labels set name=coalesce(nullif($2,''),name),color=coalesce(nullif($3,''),color),description=coalesce(nullif($4,''),description),updated_at=now() where id=$1 returning id::text,name,color,description`, r.PathValue("id"), in.Name, in.Color, in.Description), "id", "name", "color", "description")
}

func (a *App) deleteLabel(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireObjectRole(w, r, "labels", r.PathValue("id"), RoleMember); !ok {
		return
	}
	_, err := a.db.Exec(r.Context(), `delete from labels where id=$1`, r.PathValue("id"))
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"deleted": true})
}

func (a *App) listIssueLabels(w http.ResponseWriter, r *http.Request) {
	issueWS, err := a.issueWorkspace(r.Context(), r.PathValue("id"))
	if err != nil || a.requireWorkspaceRole(r, issueWS, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	rows, err := a.db.Query(r.Context(), `select l.id::text,l.name,l.color,l.description from issue_labels il join labels l on l.id=il.label_id where il.issue_id=$1 order by l.name`, r.PathValue("id"))
	writeRows(w, rows, err, "id", "name", "color", "description")
}

func (a *App) attachIssueLabel(w http.ResponseWriter, r *http.Request) {
	issueWS, err := a.issueWorkspace(r.Context(), r.PathValue("id"))
	if err != nil || a.requireWorkspaceRole(r, issueWS, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	var in struct{ LabelID string }
	if !readJSON(w, r, &in) {
		return
	}
	labelWS, err := a.objectWorkspace(r.Context(), "labels", in.LabelID)
	if err != nil || labelWS != issueWS {
		writeError(w, r, 403, "forbidden")
		return
	}
	_, err = a.db.Exec(r.Context(), `insert into issue_labels(issue_id,label_id) values($1,$2) on conflict do nothing`, r.PathValue("id"), in.LabelID)
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) detachIssueLabel(w http.ResponseWriter, r *http.Request) {
	issueWS, err := a.issueWorkspace(r.Context(), r.PathValue("id"))
	if err != nil || a.requireWorkspaceRole(r, issueWS, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	_, err = a.db.Exec(r.Context(), `delete from issue_labels where issue_id=$1 and label_id=$2`, r.PathValue("id"), r.PathValue("labelId"))
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"deleted": true})
}

func (a *App) listIssueSubscribers(w http.ResponseWriter, r *http.Request) {
	issueWS, err := a.issueWorkspace(r.Context(), r.PathValue("id"))
	if err != nil || a.requireWorkspaceRole(r, issueWS, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	rows, err := a.db.Query(r.Context(), `select subscriber_type,subscriber_id::text,created_at from issue_subscribers where issue_id=$1 order by created_at`, r.PathValue("id"))
	writeRows(w, rows, err, "type", "id", "created_at")
}

func (a *App) subscribeIssue(w http.ResponseWriter, r *http.Request) {
	issueWS, err := a.issueWorkspace(r.Context(), r.PathValue("id"))
	if err != nil || a.requireWorkspaceRole(r, issueWS, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	uid := currentUserID(r)
	_, err = a.db.Exec(r.Context(), `insert into issue_subscribers(issue_id,subscriber_type,subscriber_id) values($1,'user',$2) on conflict do nothing`, r.PathValue("id"), uid)
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"subscribed": true})
}

func (a *App) unsubscribeIssue(w http.ResponseWriter, r *http.Request) {
	issueWS, err := a.issueWorkspace(r.Context(), r.PathValue("id"))
	if err != nil || a.requireWorkspaceRole(r, issueWS, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	uid := currentUserID(r)
	_, err = a.db.Exec(r.Context(), `delete from issue_subscribers where issue_id=$1 and subscriber_type='user' and subscriber_id=$2`, r.PathValue("id"), uid)
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"subscribed": false})
}

func (a *App) addIssueReaction(w http.ResponseWriter, r *http.Request) {
	issueWS, err := a.issueWorkspace(r.Context(), r.PathValue("id"))
	if err != nil || a.requireWorkspaceRole(r, issueWS, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	var in struct{ Emoji string }
	if !readJSON(w, r, &in) {
		return
	}
	uid := currentUserID(r)
	_, err = a.db.Exec(r.Context(), `insert into issue_reactions(issue_id,actor_type,actor_id,emoji) values($1,'user',$2,$3) on conflict do nothing`, r.PathValue("id"), uid, in.Emoji)
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) removeIssueReaction(w http.ResponseWriter, r *http.Request) {
	issueWS, err := a.issueWorkspace(r.Context(), r.PathValue("id"))
	if err != nil || a.requireWorkspaceRole(r, issueWS, RoleMember) != nil {
		writeError(w, r, 403, "forbidden")
		return
	}
	emoji := r.URL.Query().Get("emoji")
	uid := currentUserID(r)
	_, err = a.db.Exec(r.Context(), `delete from issue_reactions where issue_id=$1 and actor_type='user' and actor_id=$2 and emoji=$3`, r.PathValue("id"), uid, emoji)
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"deleted": true})
}
