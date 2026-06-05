package server

import (
	"context"
	"fmt"
	"time"
)

func (a *App) executeTodoAction(ctx context.Context, ws, action string, input map[string]any) (map[string]any, error) {
	id := stringAny(input["id"])
	switch action {
	case "todo.create":
		title := stringAny(input["title"])
		if title == "" {
			return nil, fmt.Errorf("title required")
		}
		state := map[string]any{"status": TodoOpen, "source": "action"}
		for _, k := range []string{"due_at", "remind_at", "priority", "source"} {
			if v := stringAny(input[k]); v != "" {
				state[k] = v
			}
		}
		var itemID string
		err := a.db.QueryRow(ctx, `insert into items(workspace_id,kind,external_id,title,body,state,tags) values($1,'todo.item',$2,$3,$4,$5,$6) returning id::text`, ws, "todo:"+randHex(12), title, stringAny(input["body"]), mustJSON(state), mustJSON(stringSlice(input["tags"]))).Scan(&itemID)
		return map[string]any{"id": itemID}, err
	case "todo.update":
		if id == "" {
			return nil, fmt.Errorf("id required")
		}
		state := map[string]any{}
		for _, k := range []string{"due_at", "remind_at", "priority", "source"} {
			if v := stringAny(input[k]); v != "" {
				state[k] = v
			}
		}
		_, err := a.db.Exec(ctx, `update items set title=case when $3<>'' then $3 else title end, body=case when $4<>'' then $4 else body end, state=state || $5::jsonb, tags=case when $6::jsonb <> '[]'::jsonb then $6::jsonb else tags end, updated_at=now() where id=$1 and workspace_id=$2 and kind='todo.item'`, id, ws, stringAny(input["title"]), stringAny(input["body"]), mustJSON(state), mustJSON(stringSlice(input["tags"])))
		return map[string]any{"id": id, "updated": true}, err
	case "todo.complete", "todo.reopen":
		if id == "" {
			return nil, fmt.Errorf("id required")
		}
		status := TodoCompleted
		if action == "todo.reopen" {
			status = TodoOpen
		}
		state := map[string]any{"status": status}
		if status == TodoCompleted {
			state["completed_at"] = time.Now().UTC().Format(time.RFC3339)
		}
		_, err := a.db.Exec(ctx, `update items set state=state || $3::jsonb, updated_at=now() where id=$1 and workspace_id=$2 and kind='todo.item'`, id, ws, mustJSON(state))
		return map[string]any{"id": id, "status": status}, err
	case "todo.delete":
		if id == "" {
			return nil, fmt.Errorf("id required")
		}
		_, err := a.db.Exec(ctx, `delete from items where id=$1 and workspace_id=$2 and kind='todo.item'`, id, ws)
		return map[string]any{"id": id, "deleted": true}, err
	}
	return nil, fmt.Errorf("unsupported todo action %s", action)
}
