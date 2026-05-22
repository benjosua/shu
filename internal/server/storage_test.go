package server

import "testing"

func TestSafeFileName(t *testing.T) {
	if got := safeFileName("../../secret.txt"); got != "secret.txt" {
		t.Fatalf("safeFileName stripped path = %q", got)
	}
	if got := safeFileName(""); got != "file" {
		t.Fatalf("empty safeFileName = %q", got)
	}
}
