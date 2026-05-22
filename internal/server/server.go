package server

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"shu/internal/config"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func Run() error {
	ctx := context.Background()
	cfg := config.Load()
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	if _, err := db.Exec(ctx, schema); err != nil {
		return err
	}
	if err := runMigrations(ctx, db, cfg.MigrationsDir); err != nil {
		return err
	}
	app := &App{db: db, hub: NewDaemonHub(), cfg: cfg, token: cfg.Token, addr: cfg.Addr}
	if cfg.RedisURL != "" {
		opt, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			return err
		}
		app.rdb = redis.NewClient(opt)
		_ = app.rdb.Ping(ctx).Err()
		go app.redisExecutorRelay(ctx)
	}
	mux := http.NewServeMux()
	app.routes(mux)
	srv := &http.Server{Addr: app.addr, Handler: logReq(mux)}
	go app.sweeper(ctx)
	go app.autopilotScheduler(ctx)
	go func() {
		log.Printf("server on %s", app.addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	c, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	return srv.Shutdown(c)
}

func (a *App) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.db.Ping(ctx); err != nil {
		writeError(w, r, 503, "db unhealthy: "+err.Error())
		return
	}
	if a.rdb != nil {
		if err := a.rdb.Ping(ctx).Err(); err != nil {
			writeError(w, r, 503, "redis unhealthy: "+err.Error())
			return
		}
	}
	writeJSON(w, map[string]any{"status": "ok", "redis": a.rdb != nil})
}

func (a *App) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := bearerToken(r)
		admin := a.token != "" && constantTimeEqual(tok, a.token)
		principal := a.principalFromToken(r.Context(), tok)
		if !admin && principal.UserID == "" && !a.cfg.AllowAnonymous {
			writeError(w, r, 401, "unauthorized")
			return
		}
		if principal.UserID != "" {
			r = r.WithContext(context.WithValue(r.Context(), userIDKey, principal.UserID))
		}
		next(w, r)
	}
}

func (a *App) wsID(r *http.Request) (string, error) {
	slug := r.URL.Query().Get("workspace")
	if slug == "" {
		slug = r.Header.Get("X-Workspace")
	}
	id, err := a.workspaceID(r.Context(), slug)
	if err != nil {
		return "", err
	}
	if err := a.requireWorkspaceRole(r, id, RoleMember); err != nil {
		return "", err
	}
	return id, nil
}
