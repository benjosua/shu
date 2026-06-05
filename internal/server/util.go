package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"shu/internal/config"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func InitUser(args []string) error {
	if len(args) < 1 {
		return errors.New("name required")
	}
	ctx := context.Background()
	cfg := config.Load()
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	if _, err := db.Exec(ctx, schema); err != nil {
		return err
	}
	if err := runMigrations(ctx, db, cfg.MigrationsDir); err != nil {
		return err
	}
	tok, tokenHash := newToken()
	var id string
	err = db.QueryRow(ctx, `insert into users(name,token_hash) values($1,$2) returning id::text`, args[0], tokenHash).Scan(&id)
	if err != nil {
		return err
	}
	fmt.Printf("user=%s\ntoken=%s\n", id, tok)
	return nil
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Body == nil {
		return true
	}
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid json: "+err.Error())
		return false
	}
	return true
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func writeHelperError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
func writeRow(w http.ResponseWriter, row pgx.Row, names ...string) {
	m, err := scanNamed(row, names)
	if err != nil {
		writeHelperError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, m)
}

func writeRows(w http.ResponseWriter, rows pgx.Rows, err error, names ...string) {
	if err != nil {
		writeHelperError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		m, err := scanNamed(rows, names)
		if err != nil {
			writeHelperError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		writeHelperError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, out)
}

type scanner interface {
	Scan(...any) error
}

func scanNamed(s scanner, names []string) (map[string]any, error) {
	vals := make([]any, len(names))
	ptrs := make([]any, len(names))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := s.Scan(ptrs...); err != nil {
		return nil, err
	}

	out := make(map[string]any, len(names))
	for i, name := range names {
		out[name] = vals[i]
	}
	return out, nil
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func nullUUID(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func randHex(n int) string { b := make([]byte, n); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func toInt64(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int64:
		return x
	case int:
		return int64(x)
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	}
	return 0
}
