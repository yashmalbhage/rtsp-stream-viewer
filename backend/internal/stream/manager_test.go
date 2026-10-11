package stream

import (
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestValidateURL(t *testing.T) {
	valid := []string{
		"rtsp://example.com/live",
		"  rtsp://10.0.0.5:8554/cam1  ",
		"rtsps://example.com/secure",
	}
	for _, raw := range valid {
		if _, _, err := ValidateURL(raw); err != nil {
			t.Errorf("ValidateURL(%q) unexpected error: %v", raw, err)
		}
	}

	invalid := []string{"", "example.com/live", "http://example.com/live", "file:///etc/passwd", "rtsp://", "-i rtsp://x/y"}
	for _, raw := range invalid {
		if _, _, err := ValidateURL(raw); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("ValidateURL(%q) = %v, want ErrInvalidURL", raw, err)
		}
	}
}

func TestValidateURLRedactsPassword(t *testing.T) {
	_, redacted, err := ValidateURL("rtsp://admin:hunter2@example.com/live")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(redacted, "hunter2") {
		t.Fatalf("redacted URL still contains the password: %s", redacted)
	}
}

// The ffmpeg binary does not exist here, so streams sit in their retry loop;
// that is enough to exercise sharing, limits and cleanup.
func testManager(maxStreams int) *Manager {
	return NewManager(Options{
		FFmpegPath:   "/nonexistent/ffmpeg",
		FPS:          10,
		MaxWidth:     640,
		JPEGQuality:  6,
		MaxStreams:   maxStreams,
		IdleGrace:    20 * time.Millisecond,
		StallTimeout: time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestManagerSharesStreamAndCleansUp(t *testing.T) {
	m := testManager(1)
	defer m.Close()

	st1, sub1, err := m.Subscribe("rtsp://example.com/a")
	if err != nil {
		t.Fatal(err)
	}
	st2, sub2, err := m.Subscribe("rtsp://example.com/a")
	if err != nil {
		t.Fatal(err)
	}
	if st1 != st2 {
		t.Fatal("two viewers of the same URL must share one stream")
	}
	if _, _, err := m.Subscribe("rtsp://example.com/b"); !errors.Is(err, ErrTooManyStreams) {
		t.Fatalf("got %v, want ErrTooManyStreams", err)
	}

	select {
	case st := <-sub1.Status:
		if st.State == StateLive {
			t.Fatalf("unexpected live status without ffmpeg")
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber never received a status")
	}

	m.Unsubscribe(st1, sub1)
	if got := len(m.Snapshot()); got != 1 {
		t.Fatalf("stream must stay while a viewer remains, have %d streams", got)
	}
	m.Unsubscribe(st2, sub2)

	deadline := time.Now().Add(2 * time.Second)
	for len(m.Snapshot()) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("stream was not stopped after the last viewer left")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
