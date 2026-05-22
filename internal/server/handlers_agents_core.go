package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

func (a *App) createAgent(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	var in struct {
		Name, Provider, Instructions, Model string
		CustomEnv                           map[string]string
		CustomArgs                          []string
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Provider == "" {
		in.Provider = "codex"
	}
	row := a.db.QueryRow(r.Context(), `insert into agents(workspace_id,name,provider,instructions,model,custom_env,custom_args) values($1,$2,$3,$4,$5,$6,$7) returning id::text,name,provider`, ws, in.Name, in.Provider, in.Instructions, in.Model, mustJSON(in.CustomEnv), mustJSON(in.CustomArgs))
	writeRow(w, row, "id", "name", "provider")
}

func (a *App) listAgents(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	rows, err := a.db.Query(r.Context(), `select id::text,name,provider,model,archived,created_at from agents where workspace_id=$1 order by name`, ws)
	writeRows(w, rows, err, "id", "name", "provider", "model", "archived", "created_at")
}

func (a *App) resolveAgentID(ctx context.Context, ws, name string) (string, error) {
	var id string
	err := a.db.QueryRow(ctx, `select id::text from agents where workspace_id=$1 and (id::text=$2 or name=$2)`, ws, name).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("agent not found %q", name)
	}
	return id, nil
}

func (a *App) resolveAgentAny(ctx context.Context, name string) (string, error) {
	if name == "" {
		return "", errors.New("agent required")
	}
	var id string
	err := a.db.QueryRow(ctx, `select id::text from agents where id::text=$1 or name=$1 order by created_at desc limit 1`, name).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("agent not found %q", name)
	}
	return id, nil
}

func (a *App) resolveAssignee(ctx context.Context, ws, name string) (struct{ typ, id string }, error) {
	return a.resolveAssigneeForResource(ctx, ws, name, "")
}

func (a *App) resolveAssigneeForResource(ctx context.Context, ws, name, _ string) (struct{ typ, id string }, error) {
	var z struct{ typ, id string }
	if name == "" {
		return z, errors.New("assignee required")
	}
	var aid string
	if err := a.db.QueryRow(ctx, `select id::text from agents where workspace_id=$1 and (name=$2 or id::text=$2)`, ws, name).Scan(&aid); err == nil {
		z.typ = "agent"
		z.id = aid
		return z, nil
	}
	var sid string
	if err := a.db.QueryRow(ctx, `select id::text from squads where workspace_id=$1 and name=$2`, ws, name).Scan(&sid); err == nil {
		z.typ = "squad"
		z.id = sid
		return z, nil
	}
	return z, fmt.Errorf("unknown assignee %q", name)
}
