package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type Role string

const (
	RoleMember Role = "member"
	RoleAdmin  Role = "admin"
	RoleOwner  Role = "owner"
)

func roleRank(role string) int {
	switch Role(role) {
	case RoleOwner:
		return 3
	case RoleAdmin:
		return 2
	case RoleMember:
		return 1
	default:
		return 0
	}
}

func (a *App) workspaceID(ctx context.Context, slugOrID string) (string, error) {
	if slugOrID == "" {
		return "", errors.New("workspace required")
	}
	var id string
	err := a.db.QueryRow(ctx, `select id::text from workspaces where slug=$1 or id::text=$1`, slugOrID).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("workspace not found")
	}
	return id, nil
}

func (a *App) roleForWorkspace(ctx context.Context, userID, workspaceID string) (string, error) {
	var role string
	err := a.db.QueryRow(ctx, `select role from workspace_members where user_id=$1 and workspace_id=$2`, userID, workspaceID).Scan(&role)
	return role, err
}

func (a *App) requireWorkspaceRole(r *http.Request, workspaceID string, min Role) error {
	uid := currentUserID(r)
	if uid == "" {
		return nil // server token/admin mode
	}
	role, err := a.roleForWorkspace(r.Context(), uid, workspaceID)
	if err != nil || roleRank(role) < roleRank(string(min)) {
		return errors.New("forbidden")
	}
	return nil
}

func bearerToken(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

func currentUserID(r *http.Request) string {
	v, _ := r.Context().Value(userIDKey).(string)
	return v
}

func (a *App) issueWorkspace(ctx context.Context, issueID string) (string, error) {
	var ws string
	err := a.db.QueryRow(ctx, `select workspace_id::text from issues where id=$1`, issueID).Scan(&ws)
	return ws, err
}

func (a *App) attachmentWorkspace(ctx context.Context, attachmentID string) (string, error) {
	var ws string
	err := a.db.QueryRow(ctx, `select workspace_id::text from attachments where id=$1`, attachmentID).Scan(&ws)
	return ws, err
}

func (a *App) withWorkspaceRole(min Role, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ws, err := a.wsID(r)
		if err != nil {
			writeError(w, r, 400, err.Error())
			return
		}
		if err := a.requireWorkspaceRole(r, ws, min); err != nil {
			writeError(w, r, 403, "forbidden")
			return
		}
		next(w, r)
	}
}

func (a *App) objectWorkspace(ctx context.Context, table, id string) (string, error) {
	allowed := map[string]bool{
		"labels": true, "skills": true, "squads": true, "autopilots": true, "resources": true, "runs": true,
		"chat_sessions": true, "inbox_items": true, "items": true, "external_actions": true,
	}
	if !allowed[table] {
		return "", errors.New("invalid table")
	}
	var ws string
	err := a.db.QueryRow(ctx, fmt.Sprintf(`select workspace_id::text from %s where id=$1`, table), id).Scan(&ws)
	return ws, err
}

func (a *App) requireObjectRole(w http.ResponseWriter, r *http.Request, table, id string, min Role) (string, bool) {
	ws, err := a.objectWorkspace(r.Context(), table, id)
	if err != nil || a.requireWorkspaceRole(r, ws, min) != nil {
		writeError(w, r, 403, "forbidden")
		return "", false
	}
	return ws, true
}
