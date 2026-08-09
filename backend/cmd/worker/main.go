package main

import (
	"backend/app"
	"backend/config"
	"backend/worker"
)

func main() {
	cfg := config.LoadConfig()
	worker.Run(app.BootstrapWorker(cfg), cfg.WorkerMaxInFlight)
}
