package audio

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// transcodeTimeout limits the time ffmpeg runs for a single transcoding operation.
var transcodeTimeout = 60 * time.Second

var transcodeSlots = make(chan struct{}, 2)

// ConfigureTranscoding sets the transcoding policy once at startup.
func ConfigureTranscoding(timeout time.Duration, maxConcurrency int) {
	if timeout <= 0 || maxConcurrency < 1 {
		panic("invalid transcoding configuration")
	}
	transcodeTimeout = timeout
	transcodeSlots = make(chan struct{}, maxConcurrency)
}

// ffmpegArgs describes encoding parameters for each supported output format.
// Input always reads from stdin ("-i pipe:0") and output writes to stdout ("pipe:1"),
// so no temporary files are needed on disk.
var ffmpegArgs = map[string][]string{
	"mp3":  {"-f", "mp3", "-codec:a", "libmp3lame", "-q:a", "2"},
	"ogg":  {"-f", "ogg", "-codec:a", "libvorbis", "-q:a", "5"},
	"opus": {"-f", "opus", "-codec:a", "libopus", "-b:a", "96k"},
	"flac": {"-f", "flac", "-codec:a", "flac"},
	"aac":  {"-f", "adts", "-codec:a", "aac", "-b:a", "192k"},
	"m4a":  {"-f", "ipod", "-codec:a", "aac", "-b:a", "192k"},
	"wav":  {"-f", "wav", "-codec:a", "pcm_s16le"},
}

// CanTranscode reports whether the target format is in the configured ffmpeg list.
func CanTranscode(format string) bool {
	_, ok := ffmpegArgs[format]
	return ok
}

// Transcode converts audio data to another format using ffmpeg over a pipe.
//
// Returns an error instead of the original data on failure: sending WAV with MP3 Content-Type
// creates a file that many players refuse to open, and the user has no way to tell.
func Transcode(ctx context.Context, input []byte, format string) ([]byte, error) {
	args, ok := ffmpegArgs[format]
	if !ok {
		return nil, fmt.Errorf("unsupported transcode target format %q", format)
	}
	if len(input) == 0 {
		return nil, fmt.Errorf("no audio data to transcode")
	}

	// Waits for a slot, but only while the client is still connected: someone who has
	// disconnected has no reason to still queue for CPU.
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

// MimeType returns the Content-Type corresponding to the audio format.
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

// KnownFormats lists the formats the platform recognizes, used when probing a file on disk
// without knowing which format the Task produced.
func KnownFormats() []string {
	return []string{"wav", "mp3", "flac", "ogg", "opus", "aac", "m4a"}
}
