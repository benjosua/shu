package daemon

import (
	"github.com/google/uuid"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func detectProviders() []map[string]string {
	seen := map[string]bool{}
	var out []map[string]string
	for _, env := range os.Environ() {
		k, v, ok := strings.Cut(env, "=")
		if !ok || !strings.HasPrefix(k, "SHU_PROVIDER_") || v == "" {
			continue
		}
		name := strings.ToLower(strings.TrimPrefix(k, "SHU_PROVIDER_"))
		name = strings.ReplaceAll(name, "_", "-")
		out = append(out, map[string]string{"provider": name, "name": name, "path": v, "version": ""})
		seen[name] = true
	}
	names := []string{"codex", "claude", "cursor", "gemini", "copilot", "opencode", "openclaw", "hermes", "pi", "kimi", "kiro"}
	for _, n := range names {
		if seen[n] {
			continue
		}
		env := "SHU_PROVIDER_" + strings.ToUpper(n)
		p := os.Getenv(env)
		if p == "" {
			p, _ = exec.LookPath(n)
		}
		if p != "" {
			out = append(out, map[string]string{"provider": n, "name": n, "path": p, "version": ""})
		}
	}
	return out
}

func daemonID() string {
	dir := filepath.Join(os.Getenv("HOME"), ".shu")
	_ = os.MkdirAll(dir, 0700)
	p := filepath.Join(dir, "daemon_id")
	if b, err := os.ReadFile(p); err == nil && strings.TrimSpace(string(b)) != "" {
		return strings.TrimSpace(string(b))
	}
	id := uuid.NewString()
	_ = os.WriteFile(p, []byte(id), 0600)
	return id
}

func envDefault(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
