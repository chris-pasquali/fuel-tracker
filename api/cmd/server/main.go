package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fillwise/internal/cache"
	"fillwise/internal/collector"
	"fillwise/internal/config"
	"fillwise/internal/httpapi"
	"fillwise/internal/store"
	"fillwise/migrations"
)

func main() {
	cfg := config.Load()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	st, err := store.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer st.Close()
	if err := st.Migrate(ctx, migrations.Files); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	rdb, err := cache.Connect(cfg.RedisURL)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}

	col := collector.New(cfg, st)
	if err := col.Bootstrap(ctx); err != nil {
		log.Fatalf("bootstrap: %v", err)
	}
	col.StartLoop(ctx)

	srv := &http.Server{
		Addr:              cfg.Host + ":" + cfg.Port,
		Handler:           httpapi.New(cfg, st, rdb, col).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("fillwise api on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()
	<-ctx.Done()
	shutdown, done := context.WithTimeout(context.Background(), 10*time.Second)
	defer done()
	_ = srv.Shutdown(shutdown)
}
