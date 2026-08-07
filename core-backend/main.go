package main

import (
	"flag"
	"log"

	"core-backend/app"
	"core-backend/config"
	"core-backend/cron"
	"core-backend/web"
	"core-backend/worker"
)

// Một binary, hai chế độ. Web nhận request và xếp job; worker nhặt job ra chạy.
//
// Cùng binary thay vì hai chương trình riêng: chuỗi khởi tạo (kho, DB, Redis, manifest) phải
// giống hệt nhau ở cả hai, và tách ra hai main là tạo hai bản sao sẽ trôi khỏi nhau. Chúng
// vẫn scale độc lập được vì là hai service khác nhau trong compose.
//
// Tệp này cố ý mỏng: chọn chế độ rồi giao việc. Phần khởi tạo dùng chung nằm ở app.Bootstrap,
// hai vòng đời nằm ở web.Run và worker.Run — trước đây cả ba trộn trong một hàm main, nên
// đọc nó không trả lời được câu hỏi "worker thật ra chạy những gì".
func main() {
	modeFlag := flag.String("mode", string(app.ModeWeb), `chế độ chạy: "web" hoặc "worker"`)
	flag.Parse()

	mode, err := app.ParseMode(*modeFlag)
	if err != nil {
		log.Fatal(err)
	}

	cfg := config.LoadConfig()
	ttsClient := app.Bootstrap(cfg, mode)

	switch mode {
	case app.ModeWorker:
		worker.Run(ttsClient)
	case app.ModeCron:
		cron.Run()
	default:
		web.Run(cfg, ttsClient)
	}
}
