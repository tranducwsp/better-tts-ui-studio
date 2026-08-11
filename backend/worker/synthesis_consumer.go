package worker

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"backend/client"
	"backend/queue"
	"backend/state"
	"backend/synth"
)

// Run picks up jobs from the queue and processes them until a stop signal is received.
// maxInFlight is the cap on concurrent jobs per worker; total load is workers times this value.
//
// No ports are opened: the worker does not serve requests, and opening a listener just for
// healthcheck would create an attack surface no one uses. Its status is observable via logs and
// via the queue itself.
func Run(ttsClient *client.CoreTTSClient, maxInFlight int) {
	if maxInFlight < 1 {
		log.Fatal("WORKER_MAX_IN_FLIGHT must be greater than 0")
	}
	if state.RedisClient == nil {
		log.Fatal("Worker requires Redis to receive jobs, but REDIS_URL is not configured or unreachable.\n" +
			"Without Redis, running web mode is sufficient: it synthesizes in-process.")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := queue.EnsureGroup(ctx); err != nil {
		log.Fatalf("Failed to create queue consumer group: %v", err)
	}

	name := consumerName()

	slots := make(chan struct{}, maxInFlight)
	var wg sync.WaitGroup

	log.Printf("Worker %q ready, max %d concurrent jobs", name, maxInFlight)

	err := queue.Consume(ctx, name, func(jobCtx context.Context, job queue.Job) {
		select {
		case slots <- struct{}{}:
		case <-jobCtx.Done():
			return
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()

			// context.Background, not jobCtx: when a stop signal arrives, the running job
			// is allowed to finish instead of being cut off mid-way. The Consume loop has
			// already stopped accepting new jobs, so this is a bounded tail.
			synth.Run(context.Background(), ttsClient, job)
		}()
	})
	if err != nil {
		log.Printf("Queue read loop stopped: %v", err)
	}

	log.Println("Waiting for in-flight jobs to finish...")
	wg.Wait()
	log.Println("Worker stopped cleanly.")
}

// consumerName is the worker's identifier within the consumer group.
//
// Hostname is the container name in compose and the pod name on k8s, so it is both unique
// across replicas and traceable back to the real process when reading logs.
func consumerName() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "worker"
	}
	return name
}
