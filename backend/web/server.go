package web

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"backend/client"
	"backend/config"
	"backend/router"
	"backend/synth"
)

// localSynthDrainTimeout is the time to wait for in-process synthesis jobs during shutdown.
//
// Only meaningful in non-Redis deployments, where the web process synthesizes itself. Finite
// because waiting forever turns a restart into a process that never dies.
const localSynthDrainTimeout = 30 * time.Second

// Run opens the HTTP port and serves until a stop signal is received.
func Run(cfg *config.Config, ttsClient *client.CoreTTSClient) {
	r := router.NewRouter(cfg, ttsClient)

	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: r,

		// ReadHeaderTimeout is separate from ReadTimeout because they are fundamentally
		// different.
		//
		// ReadTimeout covers reading the body as well, and the upload cap is
		// MAX_UPLOAD_SIZE_MB (default 256 MB): a user on a slow network sending a valid
		// reference file needs minutes, so lowering it would cut off exactly the most
		// legitimate uploads.
		//
		// Headers, on the other hand, must arrive within seconds for any real client.
		// Bundling both into a single 120-second number means every connection that drips
		// one header byte can hold a goroutine for two full minutes, and that is far
		// cheaper than the password guessing the rate limiter already guards against.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       120 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Starting Universal Control Plane Backend (Go Chi) server at http://%s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Listen error: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down server gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		// Don't Fatalf: os.Exit skips all defers, and right below we still need to wait for
		// in-process synthesis jobs. Timing out on closing the listener is not a reason to
		// abandon work in progress.
		log.Printf("Server did not shut down cleanly within deadline: %v", err)
	}

	// Wait for the non-Redis fallback path to finish. Shutdown above only waits for HTTP
	// connections, but synthesis jobs returned their task_id long ago so it can't see them —
	// exiting right here would leave chunks stuck in "processing" and the user sees a job
	// that never completes.
	synth.WaitLocal(localSynthDrainTimeout)

	log.Println("Server exited successfully")
}
