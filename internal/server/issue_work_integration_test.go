package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"shu/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIntegrationFiveLeetCodeIssuesWorkersSolveEndToEnd(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	app, cleanup := newIntegrationTestApp(t, ctx, dsn)
	defer cleanup()

	mux := http.NewServeMux()
	app.routes(mux)
	doAny := func(method, path string, body any) any {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&buf).Encode(body); err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Authorization", "Bearer test-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code < 200 || rec.Code >= 300 {
			t.Fatalf("%s %s status=%d body=%s", method, path, rec.Code, rec.Body.String())
		}
		var out any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode %s %s: %v body=%s", method, path, err, rec.Body.String())
		}
		return out
	}
	do := func(method, path string, body any) map[string]any {
		t.Helper()
		out, ok := doAny(method, path, body).(map[string]any)
		if !ok {
			t.Fatalf("%s %s did not return object", method, path)
		}
		return out
	}

	type problem struct {
		Title       string
		Description string
		Result      string
	}
	problems := []problem{
		{
			Title:       "LeetCode: Two Sum",
			Description: "Given nums and target, return indices of two numbers that add to target. Implement O(n) solution and include tests for [2,7,11,15], target 9.",
			Result:      "Solved Two Sum with hash map in O(n) time / O(n) space; returns indices [0,1] for [2,7,11,15], target 9.",
		},
		{
			Title:       "LeetCode: Valid Parentheses",
			Description: "Given a string containing brackets, determine if input is valid. Use stack and test ()[]{} plus failure cases.",
			Result:      "Solved Valid Parentheses with stack; accepts ()[]{} and rejects mismatched or unclosed brackets.",
		},
		{
			Title:       "LeetCode: Merge Two Sorted Lists",
			Description: "Merge two sorted linked lists and return sorted result. Use iterative dummy-head implementation.",
			Result:      "Solved Merge Two Sorted Lists with iterative dummy head; preserves order and handles empty inputs.",
		},
		{
			Title:       "LeetCode: Best Time to Buy and Sell Stock",
			Description: "Return max profit from one buy and one sell. Track minimum price and max profit in one pass.",
			Result:      "Solved Best Time to Buy and Sell Stock in one pass; tracks min price and max profit, returns 5 for [7,1,5,3,6,4].",
		},
		{
			Title:       "LeetCode: Binary Search",
			Description: "Given sorted nums and target, return index or -1. Implement iterative O(log n) binary search.",
			Result:      "Solved Binary Search iteratively in O(log n); returns target index when present and -1 when absent.",
		},
	}

	do("POST", "/api/workspaces", map[string]string{"slug": "smoke", "name": "Smoke"})
	agent := do("POST", "/api/agents?workspace=smoke", map[string]any{"name": "leetcode-agent", "provider": "codex", "instructions": "Solve with tests", "model": "smoke-model"})

	registered := do("POST", "/api/daemon/executors/register", map[string]any{
		"workspace": "smoke",
		"daemonID":  "worker-daemon",
		"mode":      "local",
		"executors": []map[string]string{
			{"name": "worker-1", "provider": "codex", "version": "smoke"},
			{"name": "worker-2", "provider": "codex", "version": "smoke"},
			{"name": "worker-3", "provider": "codex", "version": "smoke"},
			{"name": "worker-4", "provider": "codex", "version": "smoke"},
			{"name": "worker-5", "provider": "codex", "version": "smoke"},
		},
	})
	executors := registered["executors"].([]any)
	if len(executors) != 5 {
		t.Fatalf("registered %d executors, want 5", len(executors))
	}

	issueWork := map[string]string{}
	for _, p := range problems {
		issue := do("POST", "/api/issues?workspace=smoke", map[string]string{
			"title":       p.Title,
			"description": p.Description,
			"priority":    "high",
			"assignee":    agent["id"].(string),
		})
		if issue["status"] != "todo" || issue["work_id"] == "" {
			t.Fatalf("assigned issue not queued: %#v", issue)
		}
		issueWork[issue["work_id"].(string)] = issue["id"].(string)
	}

	workerReturns := make(map[string]string, len(problems))
	for i, p := range problems {
		executor := executors[i].(map[string]any)
		executorID := executor["id"].(string)
		workerName := executor["name"].(string)
		claimed := do("POST", "/api/daemon/executors/"+executorID+"/work/claim", nil)
		workID := claimed["id"].(string)
		if issueWork[workID] == "" {
			t.Fatalf("worker %s claimed unknown work %#v", workerName, claimed)
		}
		if claimed["title"] == "" || claimed["body"] == "" {
			t.Fatalf("worker %s got bad payload: %#v", workerName, claimed)
		}
		claimedAgent := claimed["agent"].(map[string]any)
		if claimedAgent["name"] != "leetcode-agent" || claimedAgent["instructions"] != "Solve with tests" || claimedAgent["model"] != "smoke-model" {
			t.Fatalf("worker %s got wrong agent payload: %#v", workerName, claimedAgent)
		}
		do("POST", "/api/daemon/work/"+workID+"/start", map[string]string{"executor_id": executorID})
		do("POST", "/api/daemon/work/"+workID+"/artifacts", map[string]any{
			"executor_id": executorID,
			"type":        "message",
			"data": map[string]any{
				"role":    "agent",
				"content": p.Result,
			},
		})
		do("POST", "/api/daemon/work/"+workID+"/complete", map[string]string{
			"executor_id": executorID,
			"result":      p.Result,
		})
		work := do("GET", "/api/work/"+workID, nil)
		if work["status"] != "completed" || work["result"] != p.Result {
			t.Fatalf("worker %s work not completed: %#v", workerName, work)
		}
		issue := do("GET", "/api/issues/"+issueWork[workID], nil)
		if issue["status"] != "done" {
			t.Fatalf("worker %s issue not solved: %#v", workerName, issue)
		}
		artifacts := doAny("GET", "/api/work/"+workID+"/artifacts", nil).([]any)
		if len(artifacts) != 1 {
			t.Fatalf("worker %s artifact not persisted: %#v", workerName, artifacts)
		}
		workerReturns[workerName+" / "+claimed["title"].(string)] = work["result"].(string)
	}

	for workerAndIssue, result := range workerReturns {
		t.Logf("%s returned: %s", workerAndIssue, result)
	}
}

func newIntegrationTestApp(t *testing.T, ctx context.Context, dsn string) (*App, func()) {
	t.Helper()
	schemaName := fmt.Sprintf("shu_test_%d", time.Now().UnixNano())
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `create schema `+schemaName); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	admin.Close()

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schemaName + ",public"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, schema); err != nil {
		db.Close()
		t.Fatal(err)
	}
	app := &App{db: db, hub: NewDaemonHub(), cfg: config.Config{}, token: "test-token"}
	return app, func() {
		db.Close()
		admin, err := pgxpool.New(ctx, dsn)
		if err == nil {
			_, _ = admin.Exec(ctx, `drop schema if exists `+schemaName+` cascade`)
			admin.Close()
		}
	}
}
