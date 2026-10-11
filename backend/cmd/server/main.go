package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"rtsp-stream-viewer/backend/internal/api"
	"rtsp-stream-viewer/backend/internal/config"
	"rtsp-stream-viewer/backend/internal/stream"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := config.Load()

	if _, err := exec.LookPath(cfg.FFmpegPath); err != nil {
		log.Error("ffmpeg was not found; install it or set FFMPEG_PATH", "path", cfg.FFmpegPath)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	manager := stream.NewManager(stream.Options{
		FFmpegPath:   cfg.FFmpegPath,
		FPS:          cfg.FPS,
		MaxWidth:     cfg.MaxWidth,
		JPEGQuality:  cfg.JPEGQuality,
		MaxStreams:   cfg.MaxStreams,
		IdleGrace:    cfg.IdleGrace,
		StallTimeout: cfg.StallTimeout,
	}, log)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.NewServer(ctx, cfg, manager, log).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("server listening", "port", cfg.Port, "fps", cfg.FPS, "max_streams", cfg.MaxStreams,
			"demo_streams", len(cfg.DemoStreams))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	manager.Close()
}
