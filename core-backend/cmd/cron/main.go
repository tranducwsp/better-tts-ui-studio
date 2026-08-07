package main

import (
	"core-backend/app"
	"core-backend/config"
	"core-backend/cron"
)

func main() {
	cfg := config.LoadConfig()
	app.BootstrapCron(cfg)
	cron.Run()
}
