package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeTimeout = 10 * time.Second
	pongTimeout  = 40 * time.Second
	pingInterval = 15 * time.Second

	// closeRejected tells the browser the request itself was refused, so it
	// should show the error instead of reconnecting.
	closeRejected = 4000
)

// serverMessage is a JSON text message sent to the browser. Video frames are
// sent separately as binary messages, one JPEG each.
type serverMessage struct {
	Type    string `json:"type"` // "status" or "error"
	State   string `json:"state,omitempty"`
	Message string `json:"message,omitempty"`
}

// clientMessage is a control message from the browser.
type clientMessage struct {
	Type string `json:"type"` // "pause" or "resume"
}

// handleStream serves /ws/stream?url=rtsp://... for a single viewer.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade already wrote the HTTP error
	}
	defer conn.Close()

	// Browsers cannot read the HTTP status of a failed WebSocket handshake, so
	// request errors are reported over the socket after upgrading.
	st, sub, err := s.manager.Subscribe(r.URL.Query().Get("url"))
	if err != nil {
		sendJSON(conn, serverMessage{Type: "error", Message: err.Error()})
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(closeRejected, "request rejected"),
			time.Now().Add(writeTimeout))
		return
	}
	defer s.manager.Unsubscribe(st, sub)

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	var paused atomic.Bool
	go func() {
		defer cancel()
		conn.SetReadLimit(1024)
		_ = conn.SetReadDeadline(time.Now().Add(pongTimeout))
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(pongTimeout))
		})
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg clientMessage
			if json.Unmarshal(data, &msg) != nil {
				continue
			}
			switch msg.Type {
			case "pause":
				paused.Store(true)
			case "resume":
				paused.Store(false)
			}
		}
	}()

	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case status := <-sub.Status:
			msg := serverMessage{Type: "status", State: string(status.State), Message: status.Message}
			if sendJSON(conn, msg) != nil {
				return
			}
		case frame := <-sub.Frames:
			if paused.Load() {
				continue // keep draining so resume shows live video, not stale frames
			}
			_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if conn.WriteMessage(websocket.BinaryMessage, frame) != nil {
				return
			}
		case <-ping.C:
			if conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeTimeout)) != nil {
				return
			}
		}
	}
}

func sendJSON(conn *websocket.Conn, msg serverMessage) error {
	_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	return conn.WriteJSON(msg)
}
