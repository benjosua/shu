package server

import (
	"context"
	"fmt"
)

func (a *App) pickExecutor(ctx context.Context, ws, provider, resourceID string) (string, error) {
	mode := ""
	if resourceID != "" {
		var kind string
		if err := a.db.QueryRow(ctx, `select kind from resources where workspace_id=$1 and id=$2`, ws, resourceID).Scan(&kind); err != nil {
			return "", fmt.Errorf("resource not found")
		}
		mode = executorModeForResource(kind)
	}
	var id string
	err := a.db.QueryRow(ctx, `select id::text from executors where workspace_id=$1 and provider=$2 and status='online' and ($3='' or mode=$3) order by last_seen_at desc nulls last, created_at asc limit 1`, ws, provider, mode).Scan(&id)
	if err != nil {
		if mode != "" {
			return "", fmt.Errorf("no online %s executor for %s", mode, provider)
		}
		return "", fmt.Errorf("no online executor for %s", provider)
	}
	return id, nil
}

func executorModeForResource(kind string) string {
	if kind == "local_path" {
		return "local"
	}
	return ""
}

func (a *App) workWorkspace(ctx context.Context, id string) (string, error) {
	var ws string
	err := a.db.QueryRow(ctx, `select workspace_id::text from work_items where id=$1`, id).Scan(&ws)
	return ws, err
}

func (a *App) executorWorkspace(ctx context.Context, id string) (string, error) {
	var ws string
	err := a.db.QueryRow(ctx, `select workspace_id::text from executors where id=$1`, id).Scan(&ws)
	return ws, err
}

func (a *App) requireWorkExecutor(ctx context.Context, workID, executorID string) (string, error) {
	if executorID == "" {
		return "", fmt.Errorf("executor_id required")
	}
	var ws string
	err := a.db.QueryRow(ctx, `select workspace_id::text from work_items where id=$1 and executor_id=$2`, workID, executorID).Scan(&ws)
	if err != nil {
		return "", fmt.Errorf("work not assigned to executor")
	}
	return ws, nil
}
