// Package stream turns RTSP sources into JPEG frames with ffmpeg and shares
// each source between all of its viewers.
package stream

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalidURL     = errors.New("invalid RTSP URL")
	ErrTooManyStreams = errors.New("the server is already handling the maximum number of streams")
)

// Options control how sources are transcoded and when they are shut down.
type Options struct {
	FFmpegPath   string
	FPS          int
	MaxWidth     int
	JPEGQuality  int
	MaxStreams   int
	IdleGrace    time.Duration
	StallTimeout time.Duration
}

func (o Options) ffmpegArgs(rtspURL string) []string {
	return []string{
		"-hide_banner", "-loglevel", "error", "-nostdin",
		"-rtsp_transport", "tcp", // UDP is usually blocked on cloud hosts
		"-fflags", "nobuffer", "-flags", "low_delay",
		"-i", rtspURL,
		"-an", "-sn", "-dn",
		"-vf", fmt.Sprintf("fps=%d,scale='min(%d,iw)':-2", o.FPS, o.MaxWidth),
		"-c:v", "mjpeg", "-q:v", strconv.Itoa(o.JPEGQuality),
		"-f", "image2pipe", "pipe:1",
	}
}

// Manager keeps at most one Stream per RTSP URL, however many viewers it has.
type Manager struct {
	opts Options
	log  *slog.Logger

	mu      sync.Mutex
	streams map[string]*Stream
}

func NewManager(opts Options, log *slog.Logger) *Manager {
	return &Manager{opts: opts, log: log, streams: make(map[string]*Stream)}
}

// ValidateURL accepts only rtsp:// and rtsps:// URLs and returns the cleaned
// URL plus a copy that is safe to log or display.
func ValidateURL(raw string) (clean, redacted string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("%w: URL is empty", ErrInvalidURL)
	}
	if len(raw) > 2048 {
		return "", "", fmt.Errorf("%w: URL is too long", ErrInvalidURL)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("%w: could not be parsed", ErrInvalidURL)
	}
	if u.Scheme != "rtsp" && u.Scheme != "rtsps" {
		return "", "", fmt.Errorf("%w: must start with rtsp:// or rtsps://", ErrInvalidURL)
	}
	if u.Hostname() == "" {
		return "", "", fmt.Errorf("%w: host is missing", ErrInvalidURL)
	}
	return u.String(), u.Redacted(), nil
}

// Subscribe attaches a viewer to the stream for rawURL, starting ffmpeg if
// this is the first viewer.
func (m *Manager) Subscribe(rawURL string) (*Stream, *Subscriber, error) {
	clean, redacted, err := ValidateURL(rawURL)
	if err != nil {
		return nil, nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.streams[clean]
	if !ok {
		if len(m.streams) >= m.opts.MaxStreams {
			return nil, nil, ErrTooManyStreams
		}
		st = newStream(clean, redacted, m.opts, m.log)
		m.streams[clean] = st
		m.log.Info("stream started", "stream", redacted, "active_streams", len(m.streams))
	}
	if st.idleTimer != nil {
		st.idleTimer.Stop()
		st.idleTimer = nil
	}
	return st, st.addSubscriber(), nil
}

// Unsubscribe detaches a viewer. When the last viewer leaves, ffmpeg is kept
// for a short grace period so a page refresh does not restart the stream.
func (m *Manager) Unsubscribe(st *Stream, sub *Subscriber) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if st.removeSubscriber(sub) > 0 || m.streams[st.url] != st {
		return
	}
	if st.idleTimer != nil {
		st.idleTimer.Stop()
	}
	st.idleTimer = time.AfterFunc(m.opts.IdleGrace, func() { m.reap(st) })
}

func (m *Manager) reap(st *Stream) {
	m.mu.Lock()
	if m.streams[st.url] != st || st.viewerCount() > 0 {
		m.mu.Unlock()
		return
	}
	delete(m.streams, st.url)
	remaining := len(m.streams)
	m.mu.Unlock()

	st.stop()
	m.log.Info("stream stopped, no viewers left", "stream", st.redacted, "active_streams", remaining)
}

// Close stops every stream. Used on server shutdown.
func (m *Manager) Close() {
	m.mu.Lock()
	streams := make([]*Stream, 0, len(m.streams))
	for key, st := range m.streams {
		streams = append(streams, st)
		delete(m.streams, key)
	}
	m.mu.Unlock()
	for _, st := range streams {
		st.stop()
	}
}

// Info is a snapshot of one active stream for the stats endpoint.
type Info struct {
	URL           string `json:"url"`
	State         State  `json:"state"`
	Message       string `json:"message,omitempty"`
	Viewers       int    `json:"viewers"`
	Frames        uint64 `json:"frames"`
	UptimeSeconds int    `json:"uptimeSeconds"`
}

func (m *Manager) Snapshot() []Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Info, 0, len(m.streams))
	for _, st := range m.streams {
		out = append(out, st.info())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].URL < out[j].URL })
	return out
}

func (s *Stream) viewerCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.subs)
}

func (s *Stream) info() Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Info{
		URL:           s.redacted,
		State:         s.status.State,
		Message:       s.status.Message,
		Viewers:       len(s.subs),
		Frames:        s.frames.Load(),
		UptimeSeconds: int(time.Since(s.startedAt).Seconds()),
	}
}
