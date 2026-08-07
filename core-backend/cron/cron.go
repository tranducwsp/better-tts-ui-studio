package cron

import (
	"os"
	"os/signal"
	"syscall"
)

// Run giữ tiến trình sống trong khi sweeper (và các tác vụ nền khác về sau) chạy trong
// goroutine riêng. Tất cả tác vụ được khởi động từ app.Bootstrap trước khi gọi hàm này.
//
// Không có healthcheck endpoint, không có cổng nào. Trạng thái nhìn qua log và qua việc
// kho temp/ không phình lên.
func Run() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
}