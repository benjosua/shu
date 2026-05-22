package daemon

import (
	"os"
	"path/filepath"
)

func prepareCodexHome(dst string) error {
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	src := os.Getenv("CODEX_HOME")
	if src == "" {
		src = filepath.Join(os.Getenv("HOME"), ".codex")
	}
	// Copy only stable auth/config/context files. Avoid state_*.sqlite from
	// copied test homes; Codex can recreate transient state in work scope.
	for _, name := range []string{"auth.json", "config.toml", "AGENTS.md", "skills", "plugins", "memory.md"} {
		s := filepath.Join(src, name)
		if _, err := os.Stat(s); err != nil {
			continue
		}
		if err := copyPath(s, filepath.Join(dst, name)); err != nil {
			return err
		}
	}
	return nil
}
