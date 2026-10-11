// Package config reads runtime settings from environment variables.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// DemoStream is a ready-made RTSP source advertised to the frontend.
type DemoStream struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type Config struct {
	Port           string
	FFmpegPath     string
	FPS            int           // frames per second sent to browsers
	MaxWidth       int           // frames wider than this are scaled down
	JPEGQuality    int           // ffmpeg -q:v, 2 (best) to 31 (worst)
	MaxStreams     int           // max distinct RTSP sources transcoded at once
	IdleGrace      time.Duration // keep ffmpeg alive this long after the last viewer leaves
	StallTimeout   time.Duration // restart ffmpeg if no frame arrives for this long
	AllowedOrigins []string      // browser origins allowed to connect; "*" allows any
	DemoStreams    []DemoStream
}

func Load() Config {
	return Config{
		Port:           envString("PORT", "8080"),
		FFmpegPath:     envString("FFMPEG_PATH", "ffmpeg"),
		FPS:            envInt("STREAM_FPS", 15, 1, 30),
		MaxWidth:       envInt("STREAM_MAX_WIDTH", 960, 160, 3840),
		JPEGQuality:    envInt("STREAM_JPEG_QUALITY", 6, 2, 31),
		MaxStreams:     envInt("MAX_STREAMS", 8, 1, 256),
		IdleGrace:      time.Duration(envInt("IDLE_GRACE_SECONDS", 10, 0, 3600)) * time.Second,
		StallTimeout:   time.Duration(envInt("STALL_TIMEOUT_SECONDS", 15, 3, 300)) * time.Second,
		AllowedOrigins: splitList(envString("ALLOWED_ORIGINS", "*")),
		DemoStreams:    parseDemoStreams(os.Getenv("DEMO_STREAM_URLS")),
	}
}

func envString(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback, min, max int) int {
	v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil {
		return fallback
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, strings.TrimRight(part, "/"))
		}
	}
	return out
}

// parseDemoStreams parses "Name=rtsp://host/path,Other=rtsp://host/other".
func parseDemoStreams(s string) []DemoStream {
	out := []DemoStream{}
	for _, part := range strings.Split(s, ",") {
		name, url, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || name == "" || url == "" {
			continue
		}
		out = append(out, DemoStream{Name: strings.TrimSpace(name), URL: strings.TrimSpace(url)})
	}
	return out
}
