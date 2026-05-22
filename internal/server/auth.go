package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"
)

const tokenPrefix = "shu_"

func newToken() (string, string) {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	tok := tokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	return tok, hashToken(tok)
}

func hashToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}

func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

type tokenPrincipal struct {
	UserID string
	Kind   string
}

func (a *App) principalFromToken(ctx context.Context, token string) tokenPrincipal {
	token = strings.TrimSpace(token)
	if token == "" {
		return tokenPrincipal{}
	}
	hash := hashToken(token)
	var uid string
	var expires *time.Time
	if err := a.db.QueryRow(ctx, `select user_id::text, expires_at from personal_access_tokens where token_hash=$1`, hash).Scan(&uid, &expires); err == nil {
		if expires == nil || expires.After(time.Now()) {
			_, _ = a.db.Exec(ctx, `update personal_access_tokens set last_used_at=now() where token_hash=$1`, hash)
			return tokenPrincipal{UserID: uid, Kind: "pat"}
		}
	}
	if err := a.db.QueryRow(ctx, `select id::text from users where token_hash=$1`, hash).Scan(&uid); err == nil {
		return tokenPrincipal{UserID: uid, Kind: "user"}
	}
	return tokenPrincipal{}
}
