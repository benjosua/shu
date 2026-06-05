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

func (a *App) agentProvider(ctx context.Context, agentID string) (string, error) {
	var provider string
	if err := a.db.QueryRow(ctx, `select provider from agents where id=$1`, agentID).Scan(&provider); err != nil {
		return "", err
	}
	return provider, nil
}

func (a *App) resolveAssignee(ctx context.Context, ws, name string) (EntityRef, error) {
	return a.resolveAssigneeForResource(ctx, ws, name, "")
}

func (a *App) resolveAssigneeForResource(ctx context.Context, ws, name, _ string) (EntityRef, error) {
	var z EntityRef
	if name == "" {
		return z, errors.New("assignee required")
	}
	var aid string
	if err := a.db.QueryRow(ctx, `select id::text from agents where workspace_id=$1 and (name=$2 or id::text=$2)`, ws, name).Scan(&aid); err == nil {
		return ref(RoleTypeAgent, aid), nil
	}
	var sid string
	if err := a.db.QueryRow(ctx, `select id::text from squads where workspace_id=$1 and name=$2`, ws, name).Scan(&sid); err == nil {
		return ref(RoleTypeSquad, sid), nil
	}
	return z, fmt.Errorf("unknown assignee %q", name)
}

func (a *App) squadLeader(ctx context.Context, squadID string) (string, string, error) {
	var agentID, instructions string
	err := a.db.QueryRow(ctx, `select leader_agent_id::text,instructions from squads where id=$1`, squadID).Scan(&agentID, &instructions)
	return agentID, instructions, err
}

func (a *App) workProfileForAssignee(ctx context.Context, assignee EntityRef, prompt string) (string, string, string, error) {
	provider := "codex"
	agentID := ""
	switch assignee.Type {
	case RoleTypeAgent:
		agentID = assignee.ID
		p, err := a.agentProvider(ctx, agentID)
		if err != nil {
			return "", "", "", err
		}
		provider = p
	case RoleTypeSquad:
		leaderID, instructions, err := a.squadLeader(ctx, assignee.ID)
		if err != nil {
			return "", "", "", err
		}
		agentID = leaderID
		p, err := a.agentProvider(ctx, agentID)
		if err != nil {
			return "", "", "", err
		}
		provider = p
		if instructions != "" {
			prompt = "Squad instructions:\n" + instructions + "\n\n" + prompt
		}
	}
	return agentID, provider, prompt, nil
}
