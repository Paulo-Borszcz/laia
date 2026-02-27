package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/lojasmm/laia/internal/ai"
	aitools "github.com/lojasmm/laia/internal/ai/tools"
	"github.com/lojasmm/laia/internal/auth"
	"github.com/lojasmm/laia/internal/bot"
	"github.com/lojasmm/laia/internal/config"
	"github.com/lojasmm/laia/internal/glpi"
	"github.com/lojasmm/laia/internal/session"
	"github.com/lojasmm/laia/internal/store"
	"github.com/lojasmm/laia/internal/whatsapp"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := store.NewBoltStore(cfg.DataDir + "/laia.db")
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer db.Close()

	glpiClient := glpi.NewClient(cfg.NexusBaseURL, cfg.NexusAppToken, cfg.NexusAdminToken, cfg.NexusAdminProfile)
	waClient := whatsapp.NewClient(cfg.WAPhoneNumberID, cfg.WAAccessToken)

	agent := ai.NewAgent(cfg.OpenAIAPIKey, glpiClient, db, aitools.BuildRegistry)
	sessionMgr := session.NewManager()

	// Periodic cleanup of stale per-user locks to prevent memory leaks
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			sessionMgr.Cleanup(1 * time.Hour)
		}
	}()

	botHandler := bot.NewHandler(waClient, db, cfg.BaseURL, agent, sessionMgr)
	authHandler := auth.NewHandler(glpiClient, db, waClient)
	webhookHandler := whatsapp.NewWebhookHandler(cfg.WAVerifyToken, botHandler.HandleMessage)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	r.Get("/webhook", webhookHandler.HandleVerify)
	r.Post("/webhook", webhookHandler.HandleIncoming)

	r.Get("/auth/verify", authHandler.HandleVerifyPage)
	r.Post("/auth/verify", authHandler.HandleVerifySubmit)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("laia: listening on :%s", cfg.Port)
		log.Printf("laia: webhook verify token = %s", cfg.WAVerifyToken)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("laia: shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Println("laia: stopped")
}
