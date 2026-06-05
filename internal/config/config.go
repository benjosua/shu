package config

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	Addr            string
	DatabaseURL     string
	RedisURL        string
	Token           string
	SecretKey       string
	UploadRoot      string
	RepoCacheRoot   string
	WorkRoot        string
	MigrationsDir   string
	ShutdownTimeout time.Duration
	AllowAnonymous  bool
}

func Load() Config {
	home := getenv("HOME", ".")
	return Config{
		Addr:            getenv("ADDR", ":8090"),
		DatabaseURL:     getenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/shu?sslmode=disable"),
		RedisURL:        getenv("REDIS_URL", ""),
		Token:           getenv("SHU_TOKEN", ""),
		SecretKey:       getenv("SHU_SECRET_KEY", getenv("SHU_TOKEN", "dev-insecure-shu-secret")),
		UploadRoot:      getenv("SHU_UPLOAD_ROOT", home+"/.shu/uploads"),
		RepoCacheRoot:   getenv("SHU_REPO_CACHE", home+"/.shu/repo-cache"),
		WorkRoot:        getenv("SHU_WORK_ROOT", home+"/.shu/work"),
		MigrationsDir:   getenv("SHU_MIGRATIONS_DIR", "migrations"),
		ShutdownTimeout: 5 * time.Second,
		AllowAnonymous:  strings.EqualFold(getenv("SHU_ALLOW_ANON", "false"), "true"),
	}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
