package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var daemonWSUpgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

func (a *App) daemonWS(w http.ResponseWriter, r *http.Request) {
	executorIDs := splitCSV(r.URL.Query().Get("executor_ids"))
	if len(executorIDs) == 0 {
		writeError(w, r, 400, "executor_ids required")
		return
	}
	for _, id := range executorIDs {
		ws, err := a.executorWorkspace(r.Context(), id)
		if err != nil || a.requireWorkspaceRole(r, ws, RoleMember) != nil {
			writeError(w, r, 403, "forbidden")
			return
		}
	}
	conn, err := daemonWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	ch, unsubscribe := a.hub.Subscribe(executorIDs)
	defer unsubscribe()
	_ = conn.WriteJSON(Event{Type: "daemon.connected", Payload: map[string]any{"executor_ids": executorIDs}, TS: time.Now()})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-done:
			return
		case msg := <-ch:
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ping.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
