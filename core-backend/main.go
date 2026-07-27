package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"core-backend/client"
	"core-backend/config"
	"core-backend/db"
	"core-backend/router"
)

func main() {
	cfg := config.LoadConfig()

	// Create storage directory if missing
	if err := os.MkdirAll(cfg.StorageDir, 0755); err != nil {
		log.Fatalf("Failed to create storage directory '%s': %v", cfg.StorageDir, err)
	}

	// Initialize Database with Connection Pool settings
	db.InitDB(cfg)

	// Core TTS client
	ttsClient := client.NewCoreTTSClient(cfg.CoreTTSURL, cfg.TTSClientTimeout)

	// Create Router
	r := router.NewRouter(cfg, ttsClient)

	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown channel
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Starting VieNeu Core Backend (Go Chi) server at http://%s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Listen error: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down server gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited successfully")
}
