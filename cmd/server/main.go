package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bmikuska/shmu-weather-api/internal/api"
	"github.com/bmikuska/shmu-weather-api/internal/config"
	"github.com/bmikuska/shmu-weather-api/internal/shmu"
	"github.com/bmikuska/shmu-weather-api/internal/store"
	"github.com/bmikuska/shmu-weather-api/internal/syncer"
	"github.com/bmikuska/shmu-weather-api/internal/transform"
)

func main() {
	cfg := config.Load()
	logger := log.New(os.Stdout, "[shmu] ", log.LstdFlags|log.Lmsgprefix)

	st, err := store.Open(cfg.DatabasePath)
	if err != nil {
		logger.Fatalf("database: %v", err)
	}
	defer st.Close()

	if err := st.EnsureForecastRenderVersion(context.Background(), transform.ForecastRenderVersion); err != nil {
		logger.Fatalf("forecast render version: %v", err)
	}

	client := shmu.NewClient(cfg.SHMUBaseURL, cfg.SHMUDataBaseURL, cfg.HTTPTimeout)
	syn := syncer.New(cfg, client, st, logger)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go syn.Run(ctx)

	srv := api.New(st, syn, cfg)
	logger.Printf("rate limit: %d req / route / %s (per IP)", cfg.RateLimit, cfg.RateLimitWindow)
	logger.Printf("sqlite: 1 writer + %d readers; sync workers=%d rate=%d/s",
		2, cfg.SyncWorkers, cfg.SyncRatePerSec)
	if cfg.SyncForecasts {
		logger.Printf("ALADIN cron: publish times 03:45/10:45/15:45/22:45 UTC; first fetch +%s, retry every %s",
			cfg.FetchDelayAfterPublish, cfg.FetchRetryEvery)
	}
	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Printf("listening on %s", cfg.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("http: %v", err)
		}
	}()

	<-ctx.Done()
	logger.Printf("shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = httpServer.Shutdown(shutdownCtx)
}
