package main

import (
	"aihub.dev/server/internal/api"
	"aihub.dev/server/internal/clock"
	"aihub.dev/server/internal/config"
	"aihub.dev/server/internal/db"
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	c := config.Load()
	if c.DatabaseURL == "" {
		slog.Error("AIHUB_DATABASE_URL is required")
		os.Exit(1)
	}
	database, e := db.Open(c.DatabaseURL)
	if e != nil {
		slog.Error("database connection failed")
		os.Exit(1)
	}
	defer database.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if e = db.Migrate(ctx, database); e != nil {
		slog.Error("migration failed", "error", e.Error())
		os.Exit(1)
	}
	if e = api.SeedProviders(ctx, database); e != nil {
		slog.Error("provider seed failed")
		os.Exit(1)
	}
	app := api.New(database, clock.Real{}, c.WebDist)
	if c.Demo {
		if e = api.SeedDemo(ctx, database, app); e != nil {
			slog.Error("demo seed failed")
			os.Exit(1)
		}
	}
	srv := &http.Server{Addr: c.Addr, Handler: app.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		minute := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				minute++
				if minute%5 == 0 {
					if e := app.EvaluateCodexAll(ctx); e != nil {
						slog.Error("Codex rule evaluation failed")
					}
				}
				if e := app.EvaluateNotificationsAll(ctx); e != nil {
					slog.Error("rule evaluation failed")
				}
			}
		}
	}()
	go func() {
		slog.Info("AI Hub ready", "address", c.Addr, "protocol", 1)
		if e := srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			slog.Error("listen failed")
			stop()
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
}
