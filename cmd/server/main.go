package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bangla-news-aggregator/internal/config"
	"bangla-news-aggregator/internal/cron"
	"bangla-news-aggregator/internal/handler"
	"bangla-news-aggregator/internal/repository"
	"bangla-news-aggregator/internal/usecase"
)

func main() {
	cfg := config.Load()

	db, err := repository.NewPostgresDB(cfg)
	if err != nil {
		log.Fatalf("[main] database connection failed: %v", err)
	}
	log.Println("[main] connected to Postgres and ran migrations")

	// ---- Repositories ----
	articleRepo := repository.NewArticleRepository(db)
	feedRepo := repository.NewFeedRepository(db)
	adminRepo := repository.NewAdminRepository(db)
	usageRepo := repository.NewAIUsageRepository(db)

	// ---- Usecases ----
	authUC := usecase.NewAuthUsecase(adminRepo, cfg)
	articleUC := usecase.NewArticleUsecase(articleRepo)
	feedUC := usecase.NewFeedUsecase(feedRepo)

	// ---- Cron worker (RSS -> AI -> DB pipeline) ----
	worker := cron.NewWorker(cfg, feedRepo, articleRepo, usageRepo)

	cronCtx, cancelCron := context.WithCancel(context.Background())
	go worker.Start(cronCtx)

	// ---- HTTP handlers & router ----
	publicH := handler.NewPublicHandler(articleUC)
	adminH := handler.NewAdminHandler(authUC, articleUC, feedUC, worker, cfg.JWTExpiry)
	router := handler.NewRouter(authUC, publicH, adminH)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	go func() {
		log.Printf("[main] server listening on :%s (env=%s)", cfg.Port, cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[main] server failed: %v", err)
		}
	}()

	// ---- Graceful shutdown ----
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[main] shutting down gracefully...")
	cancelCron()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("[main] forced shutdown: %v", err)
	}
	log.Println("[main] shutdown complete")
}
