package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"shu/internal/apiclient"
	"shu/internal/config"
	"strings"
)

func prepareWorkEnv(item ClaimedWork, provider string) (*WorkEnv, error) {
	cfg := config.Load()
	if item.ExecutorMode == "cloud" && item.Resource.Kind == "local_path" {
		return nil, fmt.Errorf("cloud executor cannot access local_path resource")
	}
	root := filepath.Join(cfg.WorkRoot, item.WorkspaceID, item.ID)
	workspaceDir := filepath.Join(root, "workspace")
	if item.Resource.Kind == "local_path" && item.Resource.Locator != "" {
		workspaceDir = item.Resource.Locator
		root = filepath.Dir(workspaceDir)
	} else if isRepoResource(item.Resource.Kind) && item.Resource.Locator != "" {
		if err := prepareRepoWorkspace(item.Resource.Locator, workspaceDir); err != nil {
			return nil, err
		}
	}
	if item.PriorWorkDir != "" {
		workspaceDir = item.PriorWorkDir
		root = filepath.Dir(workspaceDir)
	}
	if err := os.MkdirAll(workspaceDir, 0o700); err != nil {
		return nil, err
	}
	brief := map[string]any{
		"work":  item,
		"agent": item.Agent,
		"executor": map[string]string{
			"provider": provider,
		},
	}
	briefJSON, _ := json.MarshalIndent(brief, "", "  ")
	_ = os.WriteFile(filepath.Join(workspaceDir, "work.json"), briefJSON, 0o600)
	var ag strings.Builder
	ag.WriteString("# Shu Executor Context\n\n")
	ag.WriteString("You are running as a remote coding agent for a Shu workspace.\n")
	ag.WriteString("Use the work prompt as authoritative user intent. Do not modify files unless work asks.\n")
	if strings.TrimSpace(item.Agent.Instructions) != "" {
		ag.WriteString("\n## Agent Instructions\n\n")
		ag.WriteString(item.Agent.Instructions)
		ag.WriteString("\n")
	}
	if strings.TrimSpace(item.StepInstructions) != "" {
		ag.WriteString("\n## Step Instructions\n\n")
		ag.WriteString(item.StepInstructions)
		ag.WriteString("\n")
	}
	ag.WriteString("\n## Work Metadata\n\nSee `work.json` in this directory.\n")
	ctxName := "AGENTS.md"
	if item.Resource.Kind == "local_path" && fileExists(filepath.Join(workspaceDir, ctxName)) {
		ctxName = "AGENTS.shu.md"
	}
	_ = os.WriteFile(filepath.Join(workspaceDir, ctxName), []byte(ag.String()), 0o600)

	env := map[string]string{
		"SHU_SERVER_URL":    apiclient.APIBase(),
		"SHU_TOKEN":         apiclient.Token(),
		"SHU_WORKSPACE_ID":  item.WorkspaceID,
		"SHU_AGENT_ID":      item.AgentID,
		"SHU_AGENT_NAME":    item.Agent.Name,
		"SHU_WORK_ID":       item.ID,
		"SHU_EXECUTOR_MODE": item.ExecutorMode,
	}
	if provider == "codex" {
		codexHome := filepath.Join(root, "codex-home")
		if err := prepareCodexHome(codexHome); err == nil {
			env["CODEX_HOME"] = codexHome
		}
	}
	return &WorkEnv{RootDir: root, WorkDir: workspaceDir, Env: env}, nil
}

func isRepoResource(kind string) bool {
	switch kind {
	case "git", "git_repo", "repo", "repo_url", "github_repo":
		return true
	default:
		return false
	}
}

func prepareRepoWorkspace(locator, work string) error {
	cacheRoot := config.Load().RepoCacheRoot
	sum := sha256.Sum256([]byte(locator))
	cache := filepath.Join(cacheRoot, hex.EncodeToString(sum[:])[:24])
	if _, err := os.Stat(filepath.Join(cache, ".git")); err == nil {
		cmd := exec.Command("git", "-C", cache, "pull", "--ff-only")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
	} else {
		_ = os.RemoveAll(cache)
		if err := os.MkdirAll(cacheRoot, 0o700); err != nil {
			return err
		}
		cmd := exec.Command("git", "clone", locator, cache)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	_ = os.RemoveAll(work)
	return copyPath(cache, work)
}

func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(src, path)
			target := filepath.Join(dst, rel)
			if d.IsDir() {
				return os.MkdirAll(target, 0o700)
			}
			return copyFile(path, target, 0o600)
		})
	}
	return copyFile(src, dst, 0o600)
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func mergeEnv(base []string, maps ...map[string]string) []string {
	out := append([]string{}, base...)
	seen := map[string]int{}
	for i, kv := range out {
		k, _, _ := strings.Cut(kv, "=")
		seen[k] = i
	}
	for _, m := range maps {
		for k, v := range m {
			if k == "" {
				continue
			}
			kv := k + "=" + v
			if i, ok := seen[k]; ok {
				out[i] = kv
			} else {
				seen[k] = len(out)
				out = append(out, kv)
			}
		}
	}
	return out
}
