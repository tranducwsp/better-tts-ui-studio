package audio

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// transcodeTimeout giới hạn thời gian chạy ffmpeg cho một lần chuyển mã.
var transcodeTimeout = 60 * time.Second

var transcodeSlots = make(chan struct{}, 2)

// ConfigureTranscoding đặt policy chuyển mã một lần lúc khởi động.
func ConfigureTranscoding(timeout time.Duration, maxConcurrency int) {
	if timeout <= 0 || maxConcurrency < 1 {
		panic("invalid transcoding configuration")
	}
	transcodeTimeout = timeout
	transcodeSlots = make(chan struct{}, maxConcurrency)
}

// ffmpegArgs mô tả tham số mã hoá cho từng định dạng đầu ra được hỗ trợ.
// Đầu vào luôn đọc từ stdin ("-i pipe:0") và kết quả ghi ra stdout ("pipe:1"), nên không
// cần tạo tập tin tạm trên đĩa.
var ffmpegArgs = map[string][]string{
	"mp3":  {"-f", "mp3", "-codec:a", "libmp3lame", "-q:a", "2"},
	"ogg":  {"-f", "ogg", "-codec:a", "libvorbis", "-q:a", "5"},
	"opus": {"-f", "opus", "-codec:a", "libopus", "-b:a", "96k"},
	"flac": {"-f", "flac", "-codec:a", "flac"},
	"aac":  {"-f", "adts", "-codec:a", "aac", "-b:a", "192k"},
	"m4a":  {"-f", "ipod", "-codec:a", "aac", "-b:a", "192k"},
	"wav":  {"-f", "wav", "-codec:a", "pcm_s16le"},
}

// CanTranscode cho biết định dạng đích có nằm trong danh sách ffmpeg được cấu hình không.
func CanTranscode(format string) bool {
	_, ok := ffmpegArgs[format]
	return ok
}

// Transcode chuyển đổi dữ liệu âm thanh sang định dạng khác bằng ffmpeg qua pipe.
//
// Trả về lỗi thay vì dữ liệu gốc khi thất bại: gửi WAV kèm Content-Type của MP3 sẽ tạo ra
// tập tin mà nhiều trình phát từ chối mở, và người dùng không có cách nào biết.
func Transcode(ctx context.Context, input []byte, format string) ([]byte, error) {
	args, ok := ffmpegArgs[format]
	if !ok {
		return nil, fmt.Errorf("unsupported transcode target format %q", format)
	}
	if len(input) == 0 {
		return nil, fmt.Errorf("no audio data to transcode")
	}

	// Chờ một chỗ, nhưng chỉ trong lúc client còn kết nối: ai đã bỏ đi thì không có lý do
	// để vẫn xếp hàng chờ CPU.
	select {
	case transcodeSlots <- struct{}{}:
		defer func() { <-transcodeSlots }()
	case <-ctx.Done():
		return nil, fmt.Errorf("transcode queue: %w", ctx.Err())
	}

	ctx, cancel := context.WithTimeout(ctx, transcodeTimeout)
	defer cancel()

	full := append([]string{"-hide_banner", "-loglevel", "error", "-i", "pipe:0"}, args...)
	full = append(full, "pipe:1")

	cmd := exec.CommandContext(ctx, "ffmpeg", full...)
	cmd.Stdin = bytes.NewReader(input)

	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("ffmpeg timed out after %s", transcodeTimeout)
		}
		return nil, fmt.Errorf("ffmpeg failed: %v: %s", err, stderr.String())
	}
	if out.Len() == 0 {
		return nil, fmt.Errorf("ffmpeg produced no output: %s", stderr.String())
	}

	return out.Bytes(), nil
}

// MimeType trả về Content-Type tương ứng với định dạng âm thanh.
func MimeType(format string) string {
	switch format {
	case "mp3":
		return "audio/mpeg"
	case "ogg":
		return "audio/ogg"
	case "opus":
		return "audio/opus"
	case "flac":
		return "audio/flac"
	case "aac":
		return "audio/aac"
	case "m4a":
		return "audio/mp4"
	case "wav":
		return "audio/wav"
	default:
		return "audio/" + format
	}
}

// KnownFormats liệt kê các định dạng nền tảng nhận biết, dùng khi phải dò tệp trên đĩa mà
// không biết trước Task đã sinh ra định dạng nào.
func KnownFormats() []string {
	return []string{"wav", "mp3", "flac", "ogg", "opus", "aac", "m4a"}
}
