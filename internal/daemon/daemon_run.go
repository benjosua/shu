package daemon

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"shu/internal/apiclient"
	"strings"
	"time"
)

func runOneWork(ctx context.Context, exe, provider string, work ClaimedWork) {
	_, _ = apiclient.Request("POST", "/api/daemon/work/"+work.ID+"/start", map[string]string{"executor_id": work.ExecutorID})
	env, err := prepareWorkEnv(work, provider)
	if err != nil {
		fail(work.ID, work.ExecutorID, err, "", "", "agent_error")
		return
	}
	prompt := buildWorkPrompt(work)
	_, _ = apiclient.Request("POST", "/api/daemon/work/"+work.ID+"/artifacts", map[string]any{"type": "message", "executor_id": work.ExecutorID, "data": map[string]string{"role": "user", "content": prompt}})
	cctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	cancelled := watchWorkCancellation(cctx, work.ID, work.ExecutorID, 3*time.Second)
	go func() {
		select {
		case <-cancelled:
			cancel()
		case <-cctx.Done():
		}
	}()
	cmd := exec.CommandContext(cctx, exe)
	if provider == "codex" {
		cmd.Args = []string{exe, "exec", "--skip-git-repo-check", "--sandbox", "workspace-write", "-"}
		if work.Agent.Model != "" {
			cmd.Args = []string{exe, "exec", "--skip-git-repo-check", "--sandbox", "workspace-write", "--model", work.Agent.Model, "-"}
		}
	} else {
		cmd.Args = append([]string{exe}, work.Agent.CustomArgs...)
	}
	cmd.Dir = env.WorkDir
	cmd.Env = mergeEnv(os.Environ(), env.Env, work.Agent.CustomEnv)
	cmd.Stdin = strings.NewReader(prompt)
	out, err := cmd.StdoutPipe()
	if err != nil {
		fail(work.ID, work.ExecutorID, err, "", env.WorkDir, "agent_error")
		return
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		fail(work.ID, work.ExecutorID, err, "", env.WorkDir, "agent_error")
		return
	}
	var buf strings.Builder
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		buf.WriteString(line + "\n")
		_, _ = apiclient.Request("POST", "/api/daemon/work/"+work.ID+"/artifacts", map[string]any{"type": "log", "executor_id": work.ExecutorID, "data": map[string]string{"text": line}})
		_, _ = apiclient.Request("POST", "/api/daemon/work/"+work.ID+"/artifacts", map[string]any{"type": "message", "executor_id": work.ExecutorID, "data": map[string]string{"role": "agent", "content": line}})
	}
	err = cmd.Wait()
	select {
	case <-cancelled:
		_, _ = apiclient.Request("POST", "/api/daemon/work/"+work.ID+"/artifacts", map[string]any{"type": "status", "executor_id": work.ExecutorID, "data": map[string]string{"status": "cancelled_by_server"}})
		return
	default:
	}
	if err != nil {
		fail(work.ID, work.ExecutorID, err, "", env.WorkDir, classifyFailure(err))
		return
	}
	result := buf.String()
	_, _ = apiclient.Request("POST", "/api/daemon/work/"+work.ID+"/artifacts", map[string]any{"type": "usage", "executor_id": work.ExecutorID, "data": map[string]int64{"inputTokens": int64(len(prompt) / 4), "outputTokens": int64(len(result) / 4)}})
	_, _ = apiclient.Request("POST", "/api/daemon/work/"+work.ID+"/complete", map[string]string{"result": result, "workDir": env.WorkDir, "work_dir": env.WorkDir, "executor_id": work.ExecutorID})
}

func fail(id, executorID string, err error, sessionID, workDir, reason string) {
	_, _ = apiclient.Request("POST", "/api/daemon/work/"+id+"/fail", map[string]string{"error": err.Error(), "session_id": sessionID, "work_dir": workDir, "failure_reason": reason, "executor_id": executorID})
}

func classifyFailure(err error) string {
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "429") || strings.Contains(msg, "rate limit") || strings.Contains(msg, "too many requests") {
		return "rate_limit"
	}
	if strings.Contains(msg, "quota") || strings.Contains(msg, "credit") {
		return "quota"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "agent_error"
}
