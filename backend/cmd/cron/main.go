package main

import (
	"time"

	"backend/app"
	"backend/config"
	"backend/cron"
)

func main() {
	cfg := config.LoadConfig()
	store := app.BootstrapCron(cfg)
	cron.Run(store, time.Duration(cfg.TempRetentionHours)*time.Hour, time.Duration(cfg.StaleChunkAfterMinutes)*time.Minute)
}
