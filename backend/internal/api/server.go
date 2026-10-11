// Package api exposes the HTTP and WebSocket endpoints used by the frontend.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/gorilla/websocket"

	"rtsp-stream-viewer/backend/internal/config"
	"rtsp-stream-viewer/backend/internal/stream"
)

type Server struct {
	cfg      config.Config
	manager  *stream.Manager
	log      *slog.Logger
	upgrader websocket.Upgrader
	// ctx ends on shutdown so that open WebSocket handlers exit.
	ctx context.Context
}

func NewServer(ctx context.Context, cfg config.Config, manager *stream.Manager, log *slog.Logger) *Server {
	s := &Server{cfg: cfg, manager: manager, log: log, ctx: ctx}
	s.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 64 << 10,
		CheckOrigin:     func(r *http.Request) bool { return s.originAllowed(r.Header.Get("Origin")) },
	}
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /api/config", s.handleConfig)
	mux.HandleFunc("GET /api/streams", s.handleStreams)
	mux.HandleFunc("GET /ws/stream", s.handleStream)
	return s.cors(mux)
}

func (s *Server) originAllowed(origin string) bool {
	if origin == "" || slices.Contains(s.cfg.AllowedOrigins, "*") {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return slices.Contains(s.cfg.AllowedOrigins, strings.TrimRight(u.String(), "/"))
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && s.originAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"demoStreams": s.cfg.DemoStreams,
		"maxStreams":  s.cfg.MaxStreams,
		"fps":         s.cfg.FPS,
	})
}

func (s *Server) handleStreams(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"streams": s.manager.Snapshot()})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
