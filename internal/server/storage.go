package server

import (
	"path/filepath"
	"strings"
)

func attachmentPath(root, workspaceID, id, name string) string {
	return filepath.Join(root, workspaceID, id+"-"+safeFileName(name))
}

func safeFileName(s string) string {
	s = filepath.Base(s)
	s = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == 0 {
			return '-'
		}
		return r
	}, s)
	if s == "." || s == "" {
		return "file"
	}
	return s
}
