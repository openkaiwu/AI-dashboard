package config

import (
	"flag"
	"os"
	"strings"
)

type Config struct {
	Addr        string
	DatabaseURL string
	Demo        bool
	WebDist     string
}

func Load() Config {
	c := Config{Addr: envOr("AIHUB_ADDR", "127.0.0.1:8080"), DatabaseURL: os.Getenv("AIHUB_DATABASE_URL"), Demo: os.Getenv("AIHUB_DEMO") == "true", WebDist: envOr("AIHUB_WEB_DIST", "../apps/web/dist")}
	flag.StringVar(&c.Addr, "addr", c.Addr, "HTTP listen address; use HTTPS reverse proxy for Internet")
	flag.StringVar(&c.DatabaseURL, "database", c.DatabaseURL, "PostgreSQL connection URL")
	flag.StringVar(&c.WebDist, "web", c.WebDist, "built Web directory")
	flag.BoolVar(&c.Demo, "demo", c.Demo, "seed local demo data (not production)")
	flag.Parse()
	return c
}
func envOr(k, f string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return f
}
