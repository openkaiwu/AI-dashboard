package config

import (
	"flag"
	"os"
	"strings"
)

type Config struct {
	Addr         string
	DatabasePath string
	Demo         bool
}

func Load() Config {
	cfg := Config{
		Addr:         envOr("AIHUB_ADDR", ":8080"),
		DatabasePath: envOr("AIHUB_DATABASE", "data/aihub.db"),
		Demo:         envTrue("AIHUB_DEMO"),
	}
	flag.StringVar(&cfg.Addr, "addr", cfg.Addr, "HTTP listen address")
	flag.StringVar(&cfg.DatabasePath, "database", cfg.DatabasePath, "SQLite database path")
	flag.BoolVar(&cfg.Demo, "demo", cfg.Demo, "seed demo user and sample quota data")
	flag.Parse()
	return cfg
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envTrue(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes"
}
