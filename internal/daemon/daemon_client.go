package daemon

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"shu/internal/apiclient"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type event struct {
	Type       string      `json:"type"`
	ExecutorID string      `json:"executor_id,omitempty"`
	Payload    interface{} `json:"payload,omitempty"`
}

func daemonWakeupWS(ctx context.Context, executorIDs []string, wakeups map[string]chan struct{}) {
	if len(executorIDs) == 0 {
		return
	}
	base := apiclient.APIBase()
	wsURL := strings.TrimPrefix(strings.TrimPrefix(base, "http://"), "https://")
	if strings.HasPrefix(base, "https://") {
		wsURL = "wss://" + wsURL
	} else {
		wsURL = "ws://" + wsURL
	}
	wsURL += "/api/daemon/ws?executor_ids=" + strings.Join(executorIDs, ",")
	header := http.Header{}
	if apiclient.Token() != "" {
		header.Set("Authorization", "Bearer "+apiclient.Token())
	}
	backoff := time.Second
	for ctx.Err() == nil {
		conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, header)
		if err != nil {
			log.Printf("daemon ws unavailable, polling fallback active: %v", err)
			sleepCtx(ctx, backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		for ctx.Err() == nil {
			var e event
			if err := conn.ReadJSON(&e); err != nil {
				_ = conn.Close()
				break
			}
			if e.ExecutorID == "" {
				if payload, ok := e.Payload.(map[string]any); ok {
					if id, ok := payload["executor_id"].(string); ok {
						e.ExecutorID = id
					}
				}
			}
			if ch := wakeups[e.ExecutorID]; ch != nil {
				select {
				case ch <- struct{}{}:
				default:
				}
			}
		}
	}
}

func executorLoop(ctx context.Context, rid, provider string, providers []map[string]string, wakeup <-chan struct{}) {
	exe := ""
	for _, p := range providers {
		if p["provider"] == provider {
			exe = p["path"]
			break
		}
	}
	if exe == "" {
		return
	}
	go func() {
		hb := time.NewTicker(15 * time.Second)
		defer hb.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-hb.C:
				if _, err := apiclient.Request("POST", "/api/daemon/executors/heartbeat", map[string]string{"executorID": rid, "executor_id": rid}); err != nil {
					log.Printf("heartbeat failed executor=%s err=%v", rid, err)
				}
			}
		}
	}()

	poll := time.NewTicker(5 * time.Second)
	defer poll.Stop()
	for {
		b, err := apiclient.Request("POST", "/api/daemon/executors/"+rid+"/work/claim", nil)
		if err != nil {
			log.Printf("claim failed executor=%s err=%v", rid, err)
			if !waitExecutorSignal(ctx, poll.C, wakeup) {
				return
			}
			continue
		}
		var work ClaimedWork
		_ = json.Unmarshal(b, &work)
		if work.ID == "" {
			if !waitExecutorSignal(ctx, poll.C, wakeup) {
				return
			}
			continue
		}
		runOneWork(ctx, exe, provider, work)
	}
}

func waitExecutorSignal(ctx context.Context, poll <-chan time.Time, wakeup <-chan struct{}) bool {
	select {
	case <-ctx.Done():
		return false
	case <-poll:
		return true
	case <-wakeup:
		return true
	}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func watchWorkCancellation(ctx context.Context, id, executorID string, every time.Duration) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		defer close(ch)
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				b, err := apiclient.Request("GET", "/api/daemon/work/"+id+"/status?executor_id="+executorID, nil)
				if err != nil {
					continue
				}
				var out struct{ Status string }
				_ = json.Unmarshal(b, &out)
				if out.Status == "cancelled" {
					return
				}
			}
		}
	}()
	return ch
}
