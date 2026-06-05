package server

import (
	"context"
	"encoding/json"
	"log"
	"time"
)

type Trigger struct {
	ID              string
	WorkspaceID     string
	Kind            string
	TargetRef       EntityRef
	Payload         map[string]any
	IntervalSeconds int
}

const (
	TriggerInterval      = "interval"
	TriggerConnectedSync = "connected_sync"
	TriggerReminder      = "reminder"
)

func (a *App) scheduler(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.runDueTriggers(ctx)
		}
	}
}

func (a *App) runDueTriggers(ctx context.Context) {
	triggers := make([]Trigger, 0, 32)
	for name, collect := range map[string]func(context.Context) ([]Trigger, error){
		"autopilot":      a.dueAutopilotTriggers,
		"connected_sync": a.dueConnectedSyncTriggers,
		"reminder":       a.dueReminderTriggers,
	} {
		ts, err := collect(ctx)
		if err != nil {
			log.Printf("scheduler %s: %v", name, err)
			continue
		}
		triggers = append(triggers, ts...)
	}
	for _, tr := range triggers {
		a.dispatchTrigger(ctx, tr)
	}
}

func (a *App) dispatchTrigger(ctx context.Context, tr Trigger) {
	switch tr.Kind {
	case TriggerInterval:
		_, _ = a.enqueueAutopilot(ctx, tr.TargetRef.ID, tr.Payload)
		if tr.ID != "" && tr.IntervalSeconds > 0 {
			_, _ = a.db.Exec(ctx, `update autopilot_triggers set next_run_at=now()+($2||' seconds')::interval, updated_at=now() where id=$1`, tr.ID, tr.IntervalSeconds)
		}
	case TriggerConnectedSync:
		go func(ws, id string) {
			_, _, err := a.runConnectedSync(context.Background(), ws, id)
			if err != nil {
				log.Printf("connected sync resource=%s: %v", id, err)
			}
		}(tr.WorkspaceID, tr.TargetRef.ID)
	case TriggerReminder:
		a.sendReminder(ctx, tr)
	}
}

func (a *App) dueAutopilotTriggers(ctx context.Context) ([]Trigger, error) {
	rows, err := a.db.Query(ctx, `select t.id::text,a.workspace_id::text,t.autopilot_id::text,t.interval_seconds,t.payload from autopilot_triggers t join autopilots a on a.id=t.autopilot_id where a.enabled=true and t.enabled=true and t.kind='interval' and t.next_run_at <= now()`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Trigger
	for rows.Next() {
		var tr Trigger
		var autopilotID string
		var payloadBytes []byte
		if err := rows.Scan(&tr.ID, &tr.WorkspaceID, &autopilotID, &tr.IntervalSeconds, &payloadBytes); err != nil {
			continue
		}
		_ = json.Unmarshal(payloadBytes, &tr.Payload)
		if tr.Payload == nil {
			tr.Payload = map[string]any{"source": "schedule"}
		}
		tr.Kind = TriggerInterval
		tr.TargetRef = ref(EntityAutopilot, autopilotID)
		out = append(out, tr)
	}
	return out, rows.Err()
}

func (a *App) dueConnectedSyncTriggers(ctx context.Context) ([]Trigger, error) {
	rows, err := a.db.Query(ctx, `select id::text,workspace_id::text,kind,metadata from resources where kind in ('email.account','calendar.account') and coalesce((metadata->>'enabled')::boolean,true)=true`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := time.Now()
	var out []Trigger
	for rows.Next() {
		var id, ws, kind string
		var mb []byte
		if err := rows.Scan(&id, &ws, &kind, &mb); err != nil {
			continue
		}
		var meta map[string]any
		_ = json.Unmarshal(mb, &meta)
		interval := metaInt(meta, "poll_interval_seconds", defaultPollInterval(kind))
		if interval <= 0 {
			continue
		}
		var last *time.Time
		_ = a.db.QueryRow(ctx, `select max(completed_at) from external_sync_runs where resource_id=$1 and status=$2`, id, ActionSucceeded).Scan(&last)
		if last != nil && now.Sub(*last) < time.Duration(interval)*time.Second {
			continue
		}
		out = append(out, Trigger{WorkspaceID: ws, Kind: TriggerConnectedSync, TargetRef: ref(EntityResource, id), Payload: map[string]any{"resource_kind": kind}, IntervalSeconds: interval})
	}
	return out, rows.Err()
}

func defaultPollInterval(kind string) int {
	if kind == "calendar.account" {
		return 900
	}
	return 300
}

func (a *App) dueReminderTriggers(ctx context.Context) ([]Trigger, error) {
	rows, err := a.db.Query(ctx, `
select id::text,workspace_id::text,kind,title,body,state
from items
where (
  kind='todo.item'
  and coalesce(state->>'status','open')='open'
  and state ? 'remind_at'
  and (state->>'remind_at')::timestamptz <= now()
  and coalesce(state->>'last_reminded_at','') = ''
) or (
  kind='calendar.event'
  and starts_at is not null
  and starts_at <= now() + interval '30 minutes'
  and starts_at >= now() - interval '5 minutes'
  and coalesce(state->>'last_reminded_at','') = ''
) or (
  kind='email.message'
  and coalesce((state->>'urgency')::int,0) >= 2
  and coalesce(state->>'last_reminded_at','') = ''
)
limit 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Trigger
	for rows.Next() {
		var id, ws, kind, title, body string
		var state []byte
		if err := rows.Scan(&id, &ws, &kind, &title, &body, &state); err != nil {
			continue
		}
		out = append(out, Trigger{WorkspaceID: ws, Kind: TriggerReminder, TargetRef: ref(EntityItem, id), Payload: map[string]any{"kind": kind, "title": title, "body": body}})
	}
	return out, rows.Err()
}

func (a *App) sendReminder(ctx context.Context, tr Trigger) {
	kind := stringAny(tr.Payload["kind"])
	title := stringAny(tr.Payload["title"])
	body := stringAny(tr.Payload["body"])
	input := map[string]any{"title": reminderTitle(kind, title), "body": body, "item_id": tr.TargetRef.ID, "kind": kind}
	created, err := a.createExternalAction(ctx, tr.WorkspaceID, "", tr.TargetRef.ID, "", "notify", input, ActionRunning)
	if err != nil {
		return
	}
	_ = a.createInbox(ctx, tr.WorkspaceID, "reminder", "info", reminderTitle(kind, title), string(mustJSON(input)))
	result := map[string]any{"notified": true}
	_ = a.finishExternalAction(ctx, created.ID, ActionSucceeded, result, "")
	_, _ = a.db.Exec(ctx, `update items set state=state || $2::jsonb, updated_at=now() where id=$1`, tr.TargetRef.ID, mustJSON(map[string]any{"last_reminded_at": time.Now().UTC().Format(time.RFC3339)}))
}

func reminderTitle(kind, title string) string {
	if title == "" {
		title = kind
	}
	switch kind {
	case "todo.item":
		return "Todo reminder: " + title
	case "calendar.event":
		return "Calendar reminder: " + title
	case "email.message":
		return "Urgent email: " + title
	default:
		return "Reminder: " + title
	}
}
