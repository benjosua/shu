package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (a *App) createConnectedResource(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	var in struct {
		Kind, Locator     string
		Metadata, Secrets map[string]any
	}
	if !readJSON(w, r, &in) {
		return
	}
	if !connectedKindSupported(in.Kind) {
		writeError(w, r, 400, "unsupported connected resource kind")
		return
	}
	if in.Metadata == nil {
		in.Metadata = map[string]any{}
	}
	if _, ok := in.Metadata["enabled"]; !ok {
		in.Metadata["enabled"] = true
	}
	if in.Locator == "" {
		in.Locator = defaultLocator(in.Kind, in.Secrets)
	}
	var id string
	err = a.db.QueryRow(r.Context(), `insert into resources(workspace_id,kind,locator,metadata) values($1,$2,$3,$4) returning id::text`, ws, in.Kind, in.Locator, mustJSON(in.Metadata)).Scan(&id)
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	if err := a.storeResourceSecrets(r.Context(), ws, id, in.Secrets); err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"id": id, "kind": in.Kind, "locator": in.Locator, "metadata": in.Metadata, "has_secrets": len(in.Secrets) > 0})
}

func (a *App) listConnectedResources(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	rows, err := a.db.Query(r.Context(), `select r.id::text,r.kind,r.locator,r.metadata,(s.resource_id is not null) as has_secrets,r.created_at from resources r left join resource_secrets s on s.resource_id=r.id where r.workspace_id=$1 and r.kind in ('email.account','calendar.account') order by r.created_at desc`, ws)
	writeRows(w, rows, err, "id", "kind", "locator", "metadata", "has_secrets", "created_at")
}

func (a *App) updateConnectedResource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ws, ok := a.requireConnectedResource(w, r, id, RoleAdmin)
	if !ok {
		return
	}
	var in struct {
		Locator           string
		Metadata, Secrets map[string]any
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Metadata != nil || in.Locator != "" {
		_, err := a.db.Exec(r.Context(), `update resources set locator=case when $2<>'' then $2 else locator end, metadata=case when $3::jsonb <> '{}'::jsonb then metadata || $3::jsonb else metadata end where id=$1`, id, in.Locator, mustJSON(nonNilMap(in.Metadata)))
		if err != nil {
			writeError(w, r, 500, err.Error())
			return
		}
	}
	if in.Secrets != nil {
		if err := a.storeResourceSecrets(r.Context(), ws, id, in.Secrets); err != nil {
			writeError(w, r, 500, err.Error())
			return
		}
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) deleteConnectedResource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := a.requireConnectedResource(w, r, id, RoleAdmin); !ok {
		return
	}
	if _, err := a.db.Exec(r.Context(), `update resources set metadata=metadata || '{"enabled":false}'::jsonb where id=$1`, id); err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) testConnectedResourceRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := a.requireConnectedResource(w, r, id, RoleMember); !ok {
		return
	}
	res, err := a.loadConnectedResource(r.Context(), id)
	if err != nil {
		writeError(w, r, 404, "resource not found")
		return
	}
	if err := a.testConnectedResource(r.Context(), res); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) syncConnectedResourceRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ws, ok := a.requireConnectedResource(w, r, id, RoleMember)
	if !ok {
		return
	}
	runID, result, err := a.runConnectedSync(r.Context(), ws, id)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "sync_run_id": runID, "error": err.Error(), "result": result})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "sync_run_id": runID, "result": result})
}

func (a *App) listConnectedItems(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	kind := r.URL.Query().Get("kind")
	rid := r.URL.Query().Get("resource_id")
	tag := r.URL.Query().Get("tag")
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	sql := `select id::text,coalesce(source_resource_id::text,''),kind,external_id,title,body,state,tags,starts_at,ends_at,occurred_at,synced_at,updated_at from items where workspace_id=$1`
	args := []any{ws}
	n := 2
	if kind != "" {
		sql += fmt.Sprintf(" and kind=$%d", n)
		args = append(args, kind)
		n++
	}
	if rid != "" {
		sql += fmt.Sprintf(" and source_resource_id=$%d", n)
		args = append(args, rid)
		n++
	}
	if tag != "" {
		sql += fmt.Sprintf(" and tags ? $%d", n)
		args = append(args, tag)
		n++
	}
	if q != "" {
		sql += fmt.Sprintf(" and to_tsvector('simple', coalesce(title,'') || ' ' || coalesce(body,'')) @@ plainto_tsquery('simple',$%d)", n)
		args = append(args, q)
		n++
	}
	sql += " order by coalesce(occurred_at, starts_at, updated_at) desc limit 200"
	rows, err := a.db.Query(r.Context(), sql, args...)
	writeRows(w, rows, err, "id", "resource_id", "kind", "external_id", "title", "body", "state", "tags", "starts_at", "ends_at", "occurred_at", "synced_at", "updated_at")
}

func (a *App) getConnectedItem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := a.requireObjectRole(w, r, "items", id, RoleMember); !ok {
		return
	}
	writeRow(w, a.db.QueryRow(r.Context(), `select id::text,coalesce(source_resource_id::text,''),kind,external_id,title,body,state,tags,starts_at,ends_at,occurred_at,synced_at,created_at,updated_at from items where id=$1`, id), "id", "resource_id", "kind", "external_id", "title", "body", "state", "tags", "starts_at", "ends_at", "occurred_at", "synced_at", "created_at", "updated_at")
}

func (a *App) updateConnectedItem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := a.requireObjectRole(w, r, "items", id, RoleMember); !ok {
		return
	}
	var in struct {
		State map[string]any
		Tags  []string
	}
	if !readJSON(w, r, &in) {
		return
	}
	_, err := a.db.Exec(r.Context(), `update items set state=case when $2::jsonb <> '{}'::jsonb then state || $2::jsonb else state end, tags=case when $3::jsonb <> '[]'::jsonb then $3::jsonb else tags end, updated_at=now() where id=$1`, id, mustJSON(nonNilMap(in.State)), mustJSON(in.Tags))
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) listConnectedActions(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	status := r.URL.Query().Get("status")
	sql := `select id::text,coalesce(resource_id::text,''),coalesce(item_id::text,''),coalesce(work_id::text,''),action,input,result,status,error,created_at,completed_at from external_actions where workspace_id=$1`
	args := []any{ws}
	if status != "" {
		sql += " and status=$2"
		args = append(args, status)
	}
	sql += " order by created_at desc limit 200"
	rows, err := a.db.Query(r.Context(), sql, args...)
	writeRows(w, rows, err, "id", "resource_id", "item_id", "work_id", "action", "input", "result", "status", "error", "created_at", "completed_at")
}

func (a *App) createConnectedAction(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	var in struct {
		ResourceID, ItemID, WorkID, Action string
		Input                              map[string]any
	}
	if !readJSON(w, r, &in) {
		return
	}
	if !connectedActionSupported(in.Action) {
		writeError(w, r, 400, "unsupported external action")
		return
	}
	if in.ResourceID != "" {
		if _, ok := a.requireConnectedResource(w, r, in.ResourceID, RoleMember); !ok {
			return
		}
	}
	if in.ItemID != "" {
		itemWS, ok := a.requireObjectRole(w, r, "items", in.ItemID, RoleMember)
		if !ok {
			return
		} else if itemWS != ws {
			writeError(w, r, 403, "forbidden")
			return
		}
	}
	if in.WorkID != "" {
		workWS, err := a.workWorkspace(r.Context(), in.WorkID)
		if err != nil || workWS != ws || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
			writeError(w, r, 403, "forbidden")
			return
		}
	}
	created, err := a.createExternalAction(r.Context(), ws, in.ResourceID, in.ItemID, in.WorkID, in.Action, in.Input, ActionPending)
	if err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"id": created.ID, "status": created.Status, "created_at": created.CreatedAt})
}

func (a *App) approveConnectedAction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ws, ok := a.requireObjectRole(w, r, "external_actions", id, RoleMember)
	if !ok {
		return
	}
	res, err := a.executeExternalAction(r.Context(), id)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error(), "result": res})
		return
	}
	a.publish(r.Context(), Event{Type: "external.action.updated", WorkspaceID: ws, Payload: map[string]string{"action_id": id}, TS: time.Now()})
	writeJSON(w, map[string]any{"ok": true, "result": res})
}

func (a *App) cancelConnectedAction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ws, ok := a.requireObjectRole(w, r, "external_actions", id, RoleMember)
	if !ok {
		return
	}
	var runID string
	if err := a.db.QueryRow(r.Context(), `update external_actions set status=$2, completed_at=now() where id=$1 and status in ($3,$4) returning coalesce(run_id::text,'')`, id, ActionCancelled, ActionPending, ActionApproved).Scan(&runID); err != nil {
		writeError(w, r, 500, err.Error())
		return
	}
	a.runStore().Finish(r.Context(), runID, ActionCancelled, nil, "")
	a.publish(r.Context(), Event{Type: "external.action.updated", WorkspaceID: ws, Payload: map[string]string{"action_id": id}, TS: time.Now()})
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) listConnectedSyncRuns(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	rows, err := a.db.Query(r.Context(), `select id::text,resource_id::text,status,items_seen,items_upserted,actions_run,error,started_at,completed_at from external_sync_runs where workspace_id=$1 order by started_at desc limit 200`, ws)
	writeRows(w, rows, err, "id", "resource_id", "status", "items_seen", "items_upserted", "actions_run", "error", "started_at", "completed_at")
}

func (a *App) storeResourceSecrets(ctx context.Context, ws, id string, secrets map[string]any) error {
	if secrets == nil {
		secrets = map[string]any{}
	}
	cipher, err := a.encryptSecret(mustJSON(secrets))
	if err != nil {
		return err
	}
	_, err = a.db.Exec(ctx, `insert into resource_secrets(resource_id,workspace_id,ciphertext) values($1,$2,$3) on conflict(resource_id) do update set ciphertext=excluded.ciphertext,updated_at=now()`, id, ws, cipher)
	return err
}

func (a *App) requireConnectedResource(w http.ResponseWriter, r *http.Request, id string, role Role) (string, bool) {
	ws, ok := a.requireObjectRole(w, r, "resources", id, role)
	if !ok {
		return "", false
	}
	var kind string
	if err := a.db.QueryRow(r.Context(), `select kind from resources where id=$1`, id).Scan(&kind); err != nil || !connectedKindSupported(kind) {
		writeError(w, r, 404, "connected resource not found")
		return "", false
	}
	return ws, true
}

func defaultLocator(kind string, secrets map[string]any) string {
	if secrets == nil {
		return kind
	}
	switch kind {
	case "email.account":
		return "email://" + stringAny(secrets["imap_user"])
	case "calendar.account":
		return stringAny(secrets["caldav_url"])
	}
	return kind
}

type externalActionCreated struct {
	ID        string
	Status    string
	RunID     string
	CreatedAt any
}

func (a *App) createExternalAction(ctx context.Context, ws, resourceID, itemID, workID, action string, input map[string]any, status string) (externalActionCreated, error) {
	if status == "" {
		status = ActionPending
	}
	var out externalActionCreated
	err := a.db.QueryRow(ctx, `insert into external_actions(workspace_id,resource_id,item_id,work_id,action,input,status) values($1,$2,$3,$4,$5,$6,$7) returning id::text,status,created_at`, ws, nullUUID(resourceID), nullUUID(itemID), nullUUID(workID), action, mustJSON(nonNilMap(input)), status).Scan(&out.ID, &out.Status, &out.CreatedAt)
	if err != nil {
		return out, err
	}
	if runID, err := a.runStore().Create(ctx, ws, "external.action", runStatusForAction(out.Status), ref(EntityAction, out.ID), map[string]any{"action": action, "input": nonNilMap(input)}); err == nil {
		out.RunID = runID
		_, _ = a.db.Exec(ctx, `update external_actions set run_id=$2 where id=$1`, out.ID, runID)
		if status == ActionRunning {
			a.runStore().Start(ctx, runID)
		}
	}
	return out, nil
}

func runStatusForAction(status string) string {
	switch status {
	case ActionPending, ActionApproved:
		return WorkQueued
	case ActionRunning:
		return WorkRunning
	case ActionSucceeded:
		return ActionSucceeded
	case ActionFailed:
		return ActionFailed
	case ActionCancelled:
		return ActionCancelled
	default:
		return status
	}
}

func (a *App) finishExternalAction(ctx context.Context, actionID, status string, result map[string]any, errText string) error {
	var runID string
	err := a.db.QueryRow(ctx, `update external_actions set status=$2,error=$3,result=$4,completed_at=now() where id=$1 returning coalesce(run_id::text,'')`, actionID, status, errText, mustJSON(nonNilMap(result))).Scan(&runID)
	if err == nil {
		a.runStore().Finish(ctx, runID, runStatusForAction(status), result, errText)
	}
	return err
}

func nonNilMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func (a *App) upsertItem(ctx context.Context, ws, resourceID, kind, externalID, title, body string, state map[string]any, tags []string, starts, ends, occurred *time.Time) (bool, error) {
	return a.itemStore().Upsert(ctx, ws, resourceID, kind, externalID, title, body, state, tags, starts, ends, occurred)
}

func (a *App) runConnectedSync(ctx context.Context, ws, resourceID string) (string, syncResult, error) {
	var runID string
	lifecycleID := ""
	if rid, err := a.runStore().Create(ctx, ws, "external.sync", ActionRunning, EntityRef{}, map[string]any{"resource_id": resourceID}); err == nil {
		lifecycleID = rid
		a.runStore().Start(ctx, rid)
	}
	if err := a.db.QueryRow(ctx, `insert into external_sync_runs(workspace_id,resource_id,run_id,status) values($1,$2,$3,$4) returning id::text`, ws, resourceID, nullUUID(lifecycleID), ActionRunning).Scan(&runID); err != nil {
		return "", syncResult{}, err
	}
	a.runStore().AttachSubject(ctx, lifecycleID, ref(EntitySyncRun, runID))
	res, err := a.syncConnectedResource(ctx, resourceID)
	status := ActionSucceeded
	errText := ""
	if err != nil {
		status = ActionFailed
		errText = err.Error()
		_ = a.createInbox(ctx, ws, "external.sync.failed", "warning", "Connected resource sync failed", errText)
	}
	_, _ = a.db.Exec(ctx, `update external_sync_runs set status=$2,items_seen=$3,items_upserted=$4,actions_run=$5,error=$6,completed_at=now() where id=$1`, runID, status, res.Seen, res.Upserted, res.Actions, errText)
	a.runStore().Finish(ctx, lifecycleID, status, map[string]any{"items_seen": res.Seen, "items_upserted": res.Upserted, "actions_run": res.Actions}, errText)
	a.publish(ctx, Event{Type: "external.sync.completed", WorkspaceID: ws, Payload: map[string]string{"resource_id": resourceID, "sync_run_id": runID, "status": status}, TS: time.Now()})
	return runID, res, err
}

func (a *App) executeExternalAction(ctx context.Context, actionID string) (map[string]any, error) {
	var ws, rid, action, runID string
	var inputBytes []byte
	err := a.db.QueryRow(ctx, `update external_actions set status=$2 where id=$1 and status in ($3,$4) returning workspace_id::text,coalesce(resource_id::text,''),action,input,coalesce(run_id::text,'')`, actionID, ActionRunning, ActionPending, ActionApproved).Scan(&ws, &rid, &action, &inputBytes, &runID)
	if err != nil {
		return nil, err
	}
	a.runStore().Start(ctx, runID)
	var input map[string]any
	_ = json.Unmarshal(inputBytes, &input)
	if input == nil {
		input = map[string]any{}
	}
	var result map[string]any
	if action == "notify" {
		result = map[string]any{"notified": true}
		_ = a.createInbox(ctx, ws, "external.notify", "info", stringAny(input["title"]), stringAny(input["body"]))
	} else {
		if strings.HasPrefix(action, "todo.") {
			result, err = a.executeTodoAction(ctx, ws, action, input)
		} else {
			if rid == "" {
				err := fmt.Errorf("resource_id required for %s", action)
				_ = a.finishExternalAction(ctx, actionID, ActionFailed, nil, err.Error())
				return nil, err
			}
			res, err := a.loadConnectedResource(ctx, rid)
			if err != nil {
				_ = a.finishExternalAction(ctx, actionID, ActionFailed, nil, err.Error())
				return nil, err
			}
			result, err = executeResourceAction(ctx, a, res, action, input)
		}
		if err != nil {
			_ = a.finishExternalAction(ctx, actionID, ActionFailed, result, err.Error())
			return result, err
		}
	}
	return result, a.finishExternalAction(ctx, actionID, ActionSucceeded, result, "")
}

func (a *App) createInbox(ctx context.Context, ws, typ, severity, title, body string) error {
	if title == "" {
		title = typ
	}
	var id string
	err := a.db.QueryRow(ctx, `insert into inbox_items(workspace_id,type,severity,title,body) values($1,$2,$3,$4,$5) returning id::text`, ws, typ, severity, title, body).Scan(&id)
	if err == nil {
		a.activityStore().Record(ctx, ws, ref("inbox_item", id), "inbox.created", EntityRef{}, map[string]any{"type": typ, "severity": severity, "title": title})
	}
	return err
}

func (a *App) handleExternalItemEvent(ctx context.Context, ws, resourceID, kind, externalID, title, body string, state map[string]any) error {
	a.publish(ctx, Event{Type: "external.item.upserted", WorkspaceID: ws, Payload: map[string]string{"resource_id": resourceID, "kind": kind, "external_id": externalID}, TS: time.Now()})
	return a.enqueueResourceEventAutopilots(ctx, ws, resourceID, kind, externalID, title, body, state)
}

func (a *App) enqueueResourceEventAutopilots(ctx context.Context, ws, resourceID, kind, externalID, title, body string, state map[string]any) error {
	rows, err := a.db.Query(ctx, `select t.autopilot_id::text,t.payload,a.prompt,a.assignee_type,a.assignee_id::text from autopilot_triggers t join autopilots a on a.id=t.autopilot_id where a.workspace_id=$1 and a.enabled=true and t.enabled=true and t.kind='resource_event'`, ws)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var autopilotID, prompt, atype, aid string
		var payloadBytes []byte
		if err := rows.Scan(&autopilotID, &payloadBytes, &prompt, &atype, &aid); err != nil {
			continue
		}
		var payload map[string]any
		_ = json.Unmarshal(payloadBytes, &payload)
		if !resourceEventMatches(payload, resourceID, kind) {
			continue
		}
		fullPrompt := prompt + "\n\nRESOURCE_EVENT:\n" + string(mustJSON(map[string]any{"resource_id": resourceID, "kind": kind, "external_id": externalID, "title": title, "body": body, "state": state}))
		agentID, provider, fullPrompt, err := a.workProfileForAssignee(ctx, ref(atype, aid), fullPrompt)
		if err != nil {
			continue
		}
		workKind := "resource.event"
		if kind == "email.message" {
			workKind = "email.triage"
		} else if kind == "calendar.event" {
			workKind = "calendar.classify"
		}
		runPayload := map[string]any{"source": "resource_event", "resource_id": resourceID, "kind": kind, "external_id": externalID}
		work, err := a.workService().Enqueue(ctx, WorkSpec{
			WorkspaceID: ws,
			Kind:        workKind,
			Title:       "Resource event: " + title,
			Prompt:      fullPrompt,
			Provider:    provider,
			AgentID:     agentID,
			Policy:      map[string]any{"resource_id": resourceID, "external_id": externalID, "kind": kind, "autopilot_id": autopilotID},
			RunKind:     "agent.resource_event",
			RunInput:    runPayload,
		})
		if err != nil {
			continue
		}
		_, _ = a.db.Exec(ctx, `insert into autopilot_runs(autopilot_id,workspace_id,status,work_id,run_id,trigger_payload) values($1,$2,$3,$4,$5,$6)`, autopilotID, ws, WorkQueued, work.WorkID, nullUUID(work.RunID), mustJSON(runPayload))
		a.publishWorkCreated(ctx, ws, work, nil)
	}
	return rows.Err()
}

func resourceEventMatches(payload map[string]any, resourceID, kind string) bool {
	if payload == nil {
		return true
	}
	if s := stringAny(payload["resource_id"]); s != "" && s != resourceID {
		return false
	}
	if s := stringAny(payload["kind"]); s != "" && s != kind {
		return false
	}
	return true
}

func (a *App) postProcessExternalArtifact(ctx context.Context, ws, workID, typ string, data map[string]any) {
	var policyBytes []byte
	if err := a.db.QueryRow(ctx, `select policy from work_items where id=$1`, workID).Scan(&policyBytes); err != nil {
		return
	}
	var policy map[string]any
	_ = json.Unmarshal(policyBytes, &policy)
	resourceID := stringAny(policy["resource_id"])
	externalID := stringAny(policy["external_id"])
	kind := stringAny(policy["kind"])
	if resourceID == "" || externalID == "" {
		return
	}
	var itemID string
	_ = a.db.QueryRow(ctx, `select id::text from items where workspace_id=$1 and source_resource_id=$2 and kind=$3 and external_id=$4`, ws, resourceID, kind, externalID).Scan(&itemID)
	if itemID == "" {
		return
	}
	switch typ {
	case "email.summary":
		_, _ = a.db.Exec(ctx, `update items set state=state || $2::jsonb, updated_at=now() where id=$1`, itemID, mustJSON(map[string]any{"summary": data["summary"]}))
	case "email.triage":
		tags := stringSlice(data["tags"])
		if len(tags) > 0 {
			_, _ = a.db.Exec(ctx, `update items set tags=$2, state=state || $3::jsonb, updated_at=now() where id=$1`, itemID, mustJSON(tags), mustJSON(data))
		}
		urg := intAny(data["urgency"], intAny(data["score"], 0))
		if urg >= 2 {
			sev := "warning"
			if urg >= 3 {
				sev = "critical"
			}
			_ = a.createInbox(ctx, ws, "email.urgent", sev, "Urgent email: "+stringAny(data["subject"]), string(mustJSON(map[string]any{"item_id": itemID, "work_id": workID, "triage": data})))
		}
		if boolAny(data["spam"]) {
			_, _ = a.createExternalAction(ctx, ws, resourceID, itemID, workID, "email.move", map[string]any{"dest": "Spam", "reason": data["reason"]}, ActionPending)
			_ = a.createInbox(ctx, ws, "email.spam.review", "info", "Review spam move", string(mustJSON(map[string]any{"item_id": itemID, "work_id": workID})))
		}
	case "email.reply_draft":
		_ = a.createInbox(ctx, ws, "email.reply_draft", "info", "Review reply draft", string(mustJSON(map[string]any{"item_id": itemID, "work_id": workID, "draft": data})))
	case "calendar.extraction":
		acts := data["actions"]
		if arr, ok := acts.([]any); ok {
			for _, v := range arr {
				if m, ok := v.(map[string]any); ok {
					act := stringAny(m["action"])
					if act != "" {
						_, _ = a.createExternalAction(ctx, ws, resourceID, itemID, workID, "calendar."+act, m, ActionPending)
						_ = a.createInbox(ctx, ws, "calendar.action.review", "info", "Review calendar action", string(mustJSON(map[string]any{"item_id": itemID, "work_id": workID, "action": m})))
					}
				}
			}
		}
	case "calendar.classification":
		_, _ = a.db.Exec(ctx, `update items set state=state || $2::jsonb, tags=case when $3::jsonb <> '[]'::jsonb then $3::jsonb else tags end, updated_at=now() where id=$1`, itemID, mustJSON(data), mustJSON(stringSlice(data["tags"])))
	}
}

var _ = pgx.ErrNoRows
