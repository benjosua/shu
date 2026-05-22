package server

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIntegrationMigrationsApply(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(ctx, schema); err != nil {
		t.Fatal(err)
	}
	if err := runMigrations(ctx, db, "migrations"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"workspace_members", "personal_access_tokens", "issues", "comments", "attachments", "work_items", "artifacts"} {
		var exists bool
		if err := db.QueryRow(ctx, `select exists(select 1 from information_schema.tables where table_schema=current_schema() and table_name=$1)`, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("table %s missing", table)
		}
	}
}
