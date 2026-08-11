package synth

import (
	"context"
	"log"
	"sync"
	"time"

	"backend/client"
	"backend/queue"
)

// inFlight counts the synthesis jobs running in this process.
//
// Only the non-Redis fallback path uses this: when a queue is present, work belongs to
// workers and the web process holds nothing to wait on.
var inFlight sync.WaitGroup

// GoLocal runs a synthesis job in this process and registers it so shutdown can wait.
//
// Exists because a bare `go synth.Run(...)` has no one watching it: http.Server.Shutdown
// only waits for open connections, and the synthesis request returned its task_id long ago
// so it is considered idle. The process exits, the goroutine dies mid-flight, and the chunk
// is left "processing" forever because UpdateChunkStatus never gets to run. Workers already
// have wg.Wait() for exactly this situation; the web path did not.
//
// context.Background, not the request context: this work outlives the request that started it.
func GoLocal(tts *client.CoreTTSClient, job queue.Job) {
	inFlight.Add(1)
	go func() {
		defer inFlight.Done()
		Run(context.Background(), tts, job)
	}()
}

// WaitLocal waits for in-flight local synthesis jobs to finish, up to timeout.
//
// Returns false when time runs out with work still pending: the caller logs and exits,
// because waiting forever turns a restart into a process that never dies.
func WaitLocal(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		inFlight.Wait()
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		log.Printf("Synthesis jobs still running after %s — exiting and letting frontend retry", timeout)
		return false
	}
}
