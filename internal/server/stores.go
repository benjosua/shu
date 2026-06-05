package server

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type WorkStore struct{ db *pgxpool.Pool }
type ItemStore struct{ db *pgxpool.Pool }
type ResourceStore struct{ db *pgxpool.Pool }
type ActivityStore struct{ db *pgxpool.Pool }
type RunStore struct{ db *pgxpool.Pool }

func (a *App) workStore() WorkStore         { return WorkStore{db: a.db} }
func (a *App) itemStore() ItemStore         { return ItemStore{db: a.db} }
func (a *App) resourceStore() ResourceStore { return ResourceStore{db: a.db} }
func (a *App) activityStore() ActivityStore { return ActivityStore{db: a.db} }
func (a *App) runStore() RunStore           { return RunStore{db: a.db} }

func (s ResourceStore) Workspace(ctx context.Context, id string) (string, error) {
	var ws string
	err := s.db.QueryRow(ctx, `select workspace_id::text from resources where id=$1`, id).Scan(&ws)
	return ws, err
}

func (s WorkStore) Workspace(ctx context.Context, id string) (string, error) {
	var ws string
	err := s.db.QueryRow(ctx, `select workspace_id::text from work_items where id=$1`, id).Scan(&ws)
	return ws, err
}

func (s ItemStore) Workspace(ctx context.Context, id string) (string, error) {
	var ws string
	err := s.db.QueryRow(ctx, `select workspace_id::text from items where id=$1`, id).Scan(&ws)
	return ws, err
}

func (s ItemStore) Upsert(ctx context.Context, ws, resourceID, kind, externalID, title, body string, state map[string]any, tags []string, starts, ends, occurred any) (bool, error) {
	if state == nil {
		state = map[string]any{}
	}
	if tags == nil {
		tags = []string{}
	}
	_, err := s.db.Exec(ctx, `insert into items(workspace_id,source_resource_id,kind,external_id,title,body,state,tags,starts_at,ends_at,occurred_at,synced_at) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,now()) on conflict(source_resource_id,kind,external_id) do update set title=excluded.title,body=excluded.body,state=items.state || excluded.state,tags=case when excluded.tags='[]'::jsonb then items.tags else excluded.tags end,starts_at=excluded.starts_at,ends_at=excluded.ends_at,occurred_at=excluded.occurred_at,synced_at=now(),updated_at=now()`, ws, nullUUID(resourceID), kind, externalID, title, body, mustJSON(state), mustJSON(tags), starts, ends, occurred)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s ActivityStore) Record(ctx context.Context, ws string, subject EntityRef, typ string, actor EntityRef, payload map[string]any) {
	if ws == "" || typ == "" {
		return
	}
	var subjectID any
	var actorID any
	if subject.ID != "" {
		subjectID = subject.ID
	}
	if actor.ID != "" {
		actorID = actor.ID
	}
	_, _ = s.db.Exec(ctx, `insert into activity_events(workspace_id,subject_type,subject_id,type,actor_type,actor_id,payload) values($1,$2,$3,$4,$5,$6,$7)`, ws, subject.Type, subjectID, typ, actor.Type, actorID, mustJSON(nonNilMap(payload)))
}

func (s RunStore) Create(ctx context.Context, ws, kind, status string, subject EntityRef, input map[string]any) (string, error) {
	if status == "" {
		status = WorkQueued
	}
	var id string
	var subjectID any
	if subject.ID != "" {
		subjectID = subject.ID
	}
	err := s.db.QueryRow(ctx, `insert into runs(workspace_id,kind,subject_type,subject_id,status,input) values($1,$2,$3,$4,$5,$6) returning id::text`, ws, kind, subject.Type, subjectID, status, mustJSON(nonNilMap(input))).Scan(&id)
	return id, err
}

func (s RunStore) AttachSubject(ctx context.Context, runID string, subject EntityRef) {
	if runID == "" || !subject.valid() {
		return
	}
	_, _ = s.db.Exec(ctx, `update runs set subject_type=$2,subject_id=$3,updated_at=now() where id=$1`, runID, subject.Type, subject.ID)
}

func (s RunStore) Finish(ctx context.Context, runID, status string, result map[string]any, errText string) {
	if runID == "" {
		return
	}
	_, _ = s.db.Exec(ctx, `update runs set status=$2,result=$3,error=$4,completed_at=now(),updated_at=now() where id=$1`, runID, status, mustJSON(nonNilMap(result)), errText)
}

func (s RunStore) Start(ctx context.Context, runID string) {
	if runID == "" {
		return
	}
	_, _ = s.db.Exec(ctx, `update runs set status=$2,started_at=coalesce(started_at,now()),updated_at=now() where id=$1`, runID, WorkRunning)
}
