package main

import (
	"backend/app"
	"backend/config"
	"backend/web"
)

func main() {
	cfg := config.LoadConfig()
	web.Run(cfg, app.BootstrapWeb(cfg))
}
