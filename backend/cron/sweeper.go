package cron

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"backend/storage"
)

// defaultInterval is the interval between two sweep passes. The file RETENTION duration is
// determined by TEMP_AUDIO_RETENTION_HOURS and passed in from bootstrap.
const defaultInterval = 1 * time.Hour

// sweepOpTimeout caps a single sweep pass, so the sweeper does not hang forever when a remote
// store is unresponsive.
const sweepOpTimeout = 2 * time.Minute

// Run runs the temp audio cleanup loop and Reconcile orphaned chunks, keeping the process alive
// until a stop signal is received.
//
// The loop is the responsibility of the cron process, not storage: schedule and resources are
// policy, while storage only provides SweepTempObjects (pure data operation). Other background
// tasks are added here as they arise.
//
// No healthcheck endpoint, no ports. Status is visible through logs and through the fact that
// the temp/ store does not grow unbounded.
func Run(store storage.Store, retention, staleAfter time.Duration) {
	sweep := func() {
		ctx, cancel := context.WithTimeout(context.Background(), sweepOpTimeout)
		defer cancel()

		n, freed, err := storage.SweepTempObjects(ctx, store, retention)
		if err != nil {
			log.Printf("Temp audio sweep failed: %v", err)
			return
		}
		if n > 0 {
			log.Printf("Sweep: deleted %d files, freed %.1f MB", n, float64(freed)/(1024*1024))
		}
	}

	go func() {
		sweep()
		ticker := time.NewTicker(defaultInterval)
		defer ticker.Stop()
		for range ticker.C {
			sweep()
		}
	}()

	go ReconcileLoop(context.Background(), staleAfter)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
}