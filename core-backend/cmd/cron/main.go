package main

import (
	"time"

	"core-backend/app"
	"core-backend/config"
	"core-backend/cron"
)

func main() {
	cfg := config.LoadConfig()
	store := app.BootstrapCron(cfg)
	cron.Run(store, time.Duration(cfg.TempRetentionHours)*time.Hour)
}
