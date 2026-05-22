package server

import (
	"shu/internal/config"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type App struct {
	db    *pgxpool.Pool
	rdb   *redis.Client
	hub   *DaemonHub
	cfg   config.Config
	token string
	addr  string
}

type Event struct {
	Type        string      `json:"type"`
	WorkspaceID string      `json:"workspace_id,omitempty"`
	ExecutorID  string      `json:"executor_id,omitempty"`
	Payload     interface{} `json:"payload,omitempty"`
	TS          time.Time   `json:"ts"`
}

type DaemonHub struct {
	mu   sync.RWMutex
	subs map[string]map[chan []byte]struct{}
}
