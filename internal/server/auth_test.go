package server

import (
	"strings"
	"testing"
)

func TestNewTokenAndHash(t *testing.T) {
	tok, h := newToken()
	if !strings.HasPrefix(tok, tokenPrefix) {
		t.Fatalf("token prefix missing: %q", tok)
	}
	if h == "" || h == tok {
		t.Fatalf("bad hash: %q", h)
	}
	if hashToken(tok) != h {
		t.Fatal("hashToken mismatch")
	}
	if !constantTimeEqual(h, hashToken(tok)) {
		t.Fatal("constantTimeEqual false for equal hashes")
	}
}

func TestRoleRank(t *testing.T) {
	if !(roleRank("owner") > roleRank("admin") && roleRank("admin") > roleRank("member")) {
		t.Fatalf("role ordering wrong")
	}
	if roleRank("unknown") != 0 {
		t.Fatalf("unknown role should rank 0")
	}
}
