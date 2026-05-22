package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os/signal"
	"shu/internal/apiclient"
	"sync"
	"syscall"
)

func Run(args []string) error {
	if len(args) < 1 || args[0] != "start" {
		return errors.New("usage: shu daemon start")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	daemonID := daemonID()
	providers := detectProviders()
	if len(providers) == 0 {
		return errors.New("no providers configured; set SHU_PROVIDER_CODEX=/path or ensure codex/claude on PATH")
	}
	reg := map[string]any{"workspace": apiclient.Workspace(), "daemonID": daemonID, "daemon_id": daemonID, "mode": envDefault("SHU_EXECUTOR_MODE", "local"), "executors": providers}
	b, err := apiclient.Request("POST", "/api/daemon/executors/register", reg)
	if err != nil {
		return err
	}
	var rr struct {
		Executors []struct{ ID, Provider, Name string }
	}
	_ = json.Unmarshal(b, &rr)
	log.Printf("registered %d executors", len(rr.Executors))
	wakeups := make(map[string]chan struct{}, len(rr.Executors))
	var executorIDs []string
	for _, rt := range rr.Executors {
		executorIDs = append(executorIDs, rt.ID)
		wakeups[rt.ID] = make(chan struct{}, 1)
	}
	go daemonWakeupWS(ctx, executorIDs, wakeups)
	var wg sync.WaitGroup
	for _, rt := range rr.Executors {
		wg.Add(1)
		go func(id, provider string) { defer wg.Done(); executorLoop(ctx, id, provider, providers, wakeups[id]) }(rt.ID, rt.Provider)
	}
	wg.Wait()
	var ids []string
	for _, rt := range rr.Executors {
		ids = append(ids, rt.ID)
	}
	_, _ = apiclient.Request("POST", "/api/daemon/executors/deregister", map[string]any{"executorIDs": ids})
	return nil
}
