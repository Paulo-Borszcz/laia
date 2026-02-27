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
	"github.com/lojasmm/laia/internal/auth"
	"github.com/lojasmm/laia/internal/bot"
	"github.com/lojasmm/laia/internal/config"
	"github.com/lojasmm/laia/internal/glpi"
	"github.com/lojasmm/laia/internal/store"
	"github.com/lojasmm/laia/internal/whatsapp"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := store.NewBoltStore("laia.db")
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer db.Close()

	glpiClient := glpi.NewClient(cfg.NexusBaseURL, cfg.NexusAppToken)
	waClient := whatsapp.NewClient(cfg.WAPhoneNumberID, cfg.WAAccessToken)

	botHandler := bot.NewHandler(waClient, glpiClient, db, cfg.BaseURL)
	authHandler := auth.NewHandler(glpiClient, db)
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Println("laia: stopped")
}
