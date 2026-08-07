package main

import (
	"core-backend/app"
	"core-backend/config"
	"core-backend/web"
)

func main() {
	cfg := config.LoadConfig()
	web.Run(cfg, app.BootstrapWeb(cfg))
}
