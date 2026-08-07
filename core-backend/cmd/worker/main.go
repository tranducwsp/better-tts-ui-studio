package main

import (
	"core-backend/app"
	"core-backend/config"
	"core-backend/worker"
)

func main() {
	cfg := config.LoadConfig()
	worker.Run(app.BootstrapWorker(cfg), cfg.WorkerMaxInFlight)
}
