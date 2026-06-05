package server

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/redis/go-redis/v9"
	"net/http"
	"strings"
	"time"
)

func (a *App) sweeper(ctx context.Context) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for range t.C {
		rows, _ := a.db.Query(ctx, `update executors set status='offline' where status='online' and last_seen_at < now()-interval '60 seconds' returning id::text, workspace_id::text`)
		for rows != nil && rows.Next() {
			var id, ws string
			_ = rows.Scan(&id, &ws)
			a.publish(ctx, Event{Type: "executor.offline", WorkspaceID: ws, ExecutorID: id, TS: time.Now()})
		}
		if rows != nil {
			rows.Close()
		}
		_, _ = a.db.Exec(ctx, `update work_items set status='failed', error='executor offline timeout', completed_at=now()
where status in ('dispatched','running') and executor_id in (select id from executors where status='offline') and coalesce(started_at,dispatched_at,created_at) < now()-interval '5 minutes'`)
	}
}

func (a *App) publish(ctx context.Context, e Event) {
	if e.TS.IsZero() {
		e.TS = time.Now()
	}
	if e.WorkspaceID != "" {
		payload := map[string]any{}
		if e.Payload != nil {
			bp, _ := json.Marshal(e.Payload)
			_ = json.Unmarshal(bp, &payload)
		}
		subject := EntityRef{}
		if id := stringAny(payload["work_id"]); id != "" {
			subject = ref(EntityWork, id)
		} else if id := stringAny(payload["resource_id"]); id != "" {
			subject = ref(EntityResource, id)
		} else if id := stringAny(payload["action_id"]); id != "" {
			subject = ref(EntityAction, id)
		}
		a.activityStore().Record(ctx, e.WorkspaceID, subject, e.Type, EntityRef{}, payload)
	}
	b, _ := json.Marshal(e)
	if e.ExecutorID != "" && a.hub != nil {
		a.hub.Send(e.ExecutorID, b)
	}
	if a.rdb != nil {
		if e.WorkspaceID != "" {
			_ = a.rdb.XAdd(ctx, &redis.XAddArgs{Stream: "events:workspace:" + e.WorkspaceID, MaxLen: 10000, Approx: true, Values: map[string]any{"json": string(b)}}).Err()
		}
		if e.ExecutorID != "" {
			_ = a.rdb.XAdd(ctx, &redis.XAddArgs{Stream: "events:executor:" + e.ExecutorID, MaxLen: 10000, Approx: true, Values: map[string]any{"json": string(b)}}).Err()
			_ = a.rdb.Publish(ctx, "events:executor:"+e.ExecutorID, string(b)).Err()
		}
	}
}

func (a *App) redisExecutorRelay(ctx context.Context) {
	if a.rdb == nil || a.hub == nil {
		return
	}
	pubsub := a.rdb.PSubscribe(ctx, "events:executor:*")
	defer pubsub.Close()
	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			rid := strings.TrimPrefix(msg.Channel, "events:executor:")
			if rid != "" {
				a.hub.Send(rid, []byte(msg.Payload))
			}
		}
	}
}

func (a *App) eventsStream(w http.ResponseWriter, r *http.Request) {
	ws, err := a.wsID(r)
	if err != nil {
		writeError(w, r, 400, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	fl, _ := w.(http.Flusher)
	last := "$"
	if a.rdb == nil {
		fmt.Fprintf(w, "event: hello\ndata: no redis configured\n\n")
		fl.Flush()
		<-r.Context().Done()
		return
	}
	for {
		res, err := a.rdb.XRead(r.Context(), &redis.XReadArgs{Streams: []string{"events:workspace:" + ws, last}, Block: 30 * time.Second, Count: 10}).Result()
		if err != nil && err != redis.Nil {
			return
		}
		for _, st := range res {
			for _, msg := range st.Messages {
				last = msg.ID
				fmt.Fprintf(w, "data: %s\n\n", msg.Values["json"])
				fl.Flush()
			}
		}
	}
}
