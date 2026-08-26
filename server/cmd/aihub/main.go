package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"aihub.dev/server/internal/api"
	"aihub.dev/server/internal/clock"
	"aihub.dev/server/internal/config"
	"aihub.dev/server/internal/db"
	"aihub.dev/server/migrations"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	cfg := config.Load()

	database, err := db.Open(cfg.DatabasePath)
	if err != nil {
		slog.Error("open database", "error", err.Error())
		os.Exit(1)
	}
	defer database.Close()

	sqlText, err := migrations.FS.ReadFile("001_init.sql")
	if err != nil {
		slog.Error("read migrations", "error", err.Error())
		os.Exit(1)
	}
	ctx := context.Background()
	if err := db.Migrate(ctx, database, string(sqlText)); err != nil {
		slog.Error("migrate", "error", err.Error())
		os.Exit(1)
	}
	if err := api.SeedProviders(ctx, database); err != nil {
		slog.Error("seed providers", "error", err.Error())
		os.Exit(1)
	}

	dist := findWebDist()
	srvAPI := api.New(database, clock.Real{}, dist)
	if cfg.Demo {
		if err := api.SeedDemo(ctx, database, srvAPI); err != nil {
			slog.Error("seed demo", "error", err.Error())
			os.Exit(1)
		}
	}

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srvAPI.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := srvAPI.EvaluateAll(context.Background()); err != nil {
				slog.Error("scheduled rule evaluation", "error", err.Error())
			}
		}
	}()

	go func() {
		slog.Info("aihub listening", "addr", cfg.Addr, "database", cfg.DatabasePath, "webdist", dist)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen", "error", err.Error())
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
}

func findWebDist() string {
	candidates := []string{
		"webdist",
		filepath.Join("server", "webdist"),
		filepath.Join(filepath.Dir(os.Args[0]), "webdist"),
	}
	for _, c := range candidates {
		if st, err := os.Stat(filepath.Join(c, "index.html")); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}
