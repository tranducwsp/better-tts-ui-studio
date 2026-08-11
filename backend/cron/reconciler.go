package cron

import (
	"context"
	"log"
	"time"

	"backend/db"

	"github.com/jackc/pgx/v5/pgtype"
)

// reconcileInterval is the interval between two orphaned chunk scan passes.
//
// Shorter than storage sweep (1 hour) because a stuck chunk directly affects user experience
// (job never finishes), whereas a leftover temp file only bloats the store.
const reconcileInterval = 5 * time.Minute

// reconcileStaleChunks calls ReconcileStaleChunks on the DB to move stale pending/processing
// chunks to error. Skips if DB is not initialized (queries nil) or on query error.
//
// This is the last-resort rescue layer: nothing outside cron will automatically transition
// an orphaned chunk that has lost its queue. Worker and web only write terminal status when
// they can reach the chunk — if the job sits in a lost stream (Redis restart) or the worker
// dies mid-flight, the DB never sees another write from enqueue until cron scans.
func reconcileStaleChunks(staleAfter time.Duration) {
	if db.Queries == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	staleBefore := pgtype.Timestamptz{Time: time.Now().Add(-staleAfter), Valid: true}
	n, err := db.Queries.ReconcileStaleChunks(ctx, staleBefore)
	if err != nil {
		log.Printf("Reconcile orphaned chunks failed: %v", err)
		return
	}
	if n > 0 {
		log.Printf("Reconcile: moved %d orphaned chunks (pending/processing) -> error", n)
	}
}

// ReconcileLoop runs the reconcile loop until ctx is cancelled.
//
// Used in cron.Run (runs as a separate goroutine). If there is no DB connection, silently skips.
func ReconcileLoop(ctx context.Context, staleAfter time.Duration) {
	if db.Queries == nil {
		return
	}

	// Run one pass immediately at startup to clean up leftover chunks from the previous run.
	reconcileStaleChunks(staleAfter)

	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reconcileStaleChunks(staleAfter)
		}
	}
}