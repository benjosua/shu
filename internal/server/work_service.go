package server

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type dbRunner interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type WorkSpec struct {
	WorkspaceID string
	Kind        string
	Title       string
	Prompt      string
	ResourceID  string
	ParentID    string
	Policy      map[string]any
	Provider    string
	AgentID     string
	ExecutorID  string
	Priority    int
	RunKind     string
	RunInput    map[string]any
}

type EnqueuedWork struct {
	WorkID     string
	RunID      string
	Kind       string
	Title      string
	Status     string
	ExecutorID string
	CreatedAt  any
}

type WorkService struct {
	app *App
	db  dbRunner
}

func (a *App) workService() WorkService {
	return WorkService{app: a, db: a.db}
}

func (a *App) workServiceWith(db dbRunner) WorkService {
	return WorkService{app: a, db: db}
}

func (s WorkService) Enqueue(ctx context.Context, spec WorkSpec) (EnqueuedWork, error) {
	if spec.Kind == "" {
		spec.Kind = "work"
	}
	if spec.Provider == "" {
		spec.Provider = "codex"
	}
	if spec.RunKind == "" {
		spec.RunKind = "agent.work"
	}
	if spec.RunInput == nil {
		spec.RunInput = map[string]any{"title": spec.Title, "kind": spec.Kind}
	}
	if spec.ExecutorID == "" {
		id, err := s.app.pickExecutor(ctx, spec.WorkspaceID, spec.Provider, spec.ResourceID)
		if err != nil {
			return EnqueuedWork{}, err
		}
		spec.ExecutorID = id
	}
	var out EnqueuedWork
	err := s.db.QueryRow(ctx, `insert into work_items(workspace_id,kind,parent_id,title,prompt,resource_id,policy,provider,agent_id,executor_id,priority)
values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) returning id::text,kind,title,status,executor_id::text,created_at`, spec.WorkspaceID, spec.Kind, nullUUID(spec.ParentID), spec.Title, spec.Prompt, nullUUID(spec.ResourceID), mustJSON(spec.Policy), spec.Provider, nullUUID(spec.AgentID), spec.ExecutorID, spec.Priority).Scan(&out.WorkID, &out.Kind, &out.Title, &out.Status, &out.ExecutorID, &out.CreatedAt)
	if err != nil {
		return EnqueuedWork{}, err
	}
	if runID, err := s.app.runStoreWith(s.db).Create(ctx, spec.WorkspaceID, spec.RunKind, WorkQueued, ref(EntityWork, out.WorkID), spec.RunInput); err == nil {
		out.RunID = runID
		_, _ = s.db.Exec(ctx, `update work_items set run_id=$2 where id=$1`, out.WorkID, runID)
	}
	return out, nil
}

func (a *App) publishWorkCreated(ctx context.Context, ws string, work EnqueuedWork, payload map[string]string) {
	if payload == nil {
		payload = map[string]string{}
	}
	payload["work_id"] = work.WorkID
	a.publish(ctx, Event{Type: "work.created", WorkspaceID: ws, ExecutorID: work.ExecutorID, Payload: payload, TS: time.Now()})
}
