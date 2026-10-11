package stream

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type State string

const (
	StateConnecting   State = "connecting"
	StateLive         State = "live"
	StateReconnecting State = "reconnecting"
)

// Status is what viewers are told about the health of the RTSP source.
type Status struct {
	State   State  `json:"state"`
	Message string `json:"message,omitempty"`
}

const (
	minBackoff = 1 * time.Second
	maxBackoff = 15 * time.Second
)

// Subscriber is one viewer of a stream. Both channels hold a single item and
// the newest value always wins, so a slow viewer drops frames instead of
// building up latency or blocking other viewers.
type Subscriber struct {
	Frames chan []byte
	Status chan Status
}

func newSubscriber() *Subscriber {
	return &Subscriber{Frames: make(chan []byte, 1), Status: make(chan Status, 1)}
}

// offer replaces whatever is waiting in ch with v. Only the stream goroutine
// (under Stream.mu) sends, so the replace cannot race with another sender.
func offer[T any](ch chan T, v T) {
	select {
	case ch <- v:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- v:
	default:
	}
}

// Stream owns one ffmpeg process pulling one RTSP URL and fans its frames out
// to every subscriber.
type Stream struct {
	url      string
	redacted string
	opts     Options
	log      *slog.Logger

	cancel context.CancelFunc
	done   chan struct{}

	startedAt time.Time
	frames    atomic.Uint64

	mu        sync.Mutex
	subs      map[*Subscriber]struct{}
	status    Status
	lastFrame []byte
	idleTimer *time.Timer
}

func newStream(url, redacted string, opts Options, log *slog.Logger) *Stream {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Stream{
		url:       url,
		redacted:  redacted,
		opts:      opts,
		log:       log.With("stream", redacted),
		cancel:    cancel,
		done:      make(chan struct{}),
		startedAt: time.Now(),
		subs:      make(map[*Subscriber]struct{}),
		status:    Status{State: StateConnecting},
	}
	go s.run(ctx)
	return s
}

func (s *Stream) stop() {
	s.cancel()
	<-s.done
}

func (s *Stream) addSubscriber() *Subscriber {
	sub := newSubscriber()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subs[sub] = struct{}{}
	// Bring the new viewer up to date immediately.
	sub.Status <- s.status
	if s.status.State == StateLive && s.lastFrame != nil {
		sub.Frames <- s.lastFrame
	}
	return sub
}

func (s *Stream) removeSubscriber(sub *Subscriber) (remaining int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subs, sub)
	return len(s.subs)
}

func (s *Stream) setStatus(st Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = st
	if st.State != StateLive {
		s.lastFrame = nil
	}
	for sub := range s.subs {
		offer(sub.Status, st)
	}
}

func (s *Stream) broadcast(frame []byte) {
	s.frames.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastFrame = frame
	for sub := range s.subs {
		offer(sub.Frames, frame)
	}
}

// run keeps ffmpeg alive for as long as the stream exists, restarting it with
// exponential backoff whenever the source fails or goes quiet.
func (s *Stream) run(ctx context.Context) {
	defer close(s.done)
	backoff := minBackoff
	for {
		gotFrames, err := s.runFFmpeg(ctx)
		if ctx.Err() != nil {
			return
		}
		if gotFrames {
			backoff = minBackoff
		}
		reason := "stream ended"
		if err != nil {
			reason = err.Error()
		}
		s.log.Warn("source failed, will retry", "reason", reason, "retry_in", backoff)
		s.setStatus(Status{
			State:   StateReconnecting,
			Message: fmt.Sprintf("%s. Retrying in %s.", reason, backoff),
		})
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

// runFFmpeg runs a single ffmpeg process until it exits, stalls or ctx ends.
func (s *Stream) runFFmpeg(ctx context.Context) (gotFrames bool, err error) {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.CommandContext(runCtx, s.opts.FFmpegPath, s.opts.ffmpegArgs(s.url)...)
	cmd.WaitDelay = 2 * time.Second
	stderr := &tailBuffer{max: 4096}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false, fmt.Errorf("could not start ffmpeg: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf("could not start ffmpeg: %w", err)
	}

	// The watchdog covers both a source that never answers and one that
	// silently stops sending video, independent of ffmpeg's own timeouts.
	var stalled atomic.Bool
	watchdog := time.AfterFunc(s.opts.StallTimeout, func() {
		stalled.Store(true)
		cancel()
	})
	defer watchdog.Stop()

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 256<<10), maxFrameSize)
	sc.Split(splitJPEG)
	for sc.Scan() {
		watchdog.Reset(s.opts.StallTimeout)
		if !gotFrames {
			gotFrames = true
			s.log.Info("source is live")
			s.setStatus(Status{State: StateLive})
		}
		// The scanner reuses its buffer, and frames are shared between viewers.
		s.broadcast(bytes.Clone(sc.Bytes()))
	}
	waitErr := cmd.Wait()

	switch {
	case ctx.Err() != nil:
		return gotFrames, ctx.Err()
	case stalled.Load() && gotFrames:
		return gotFrames, fmt.Errorf("no video received for %s", s.opts.StallTimeout)
	case stalled.Load():
		return gotFrames, errors.New("timed out connecting to the stream")
	}
	if line := stderr.lastLine(); line != "" {
		return gotFrames, errors.New(s.describe(line))
	}
	if errors.Is(sc.Err(), bufio.ErrTooLong) {
		return gotFrames, errors.New("received an oversized video frame")
	}
	if waitErr != nil {
		return gotFrames, fmt.Errorf("ffmpeg exited: %w", waitErr)
	}
	return gotFrames, nil
}

// describe turns an ffmpeg error line into something a user can act on, and
// makes sure credentials embedded in the URL never reach logs or browsers.
func (s *Stream) describe(line string) string {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized"):
		return "authentication failed (401), check the username and password"
	case strings.Contains(lower, "404") || strings.Contains(lower, "not found"):
		return "stream path not found (404)"
	case strings.Contains(lower, "connection refused"):
		return "connection refused by the stream server"
	case strings.Contains(lower, "name or service not known"),
		strings.Contains(lower, "failed to resolve"),
		strings.Contains(lower, "name does not resolve"):
		return "host not found"
	case strings.Contains(lower, "no route to host"), strings.Contains(lower, "network is unreachable"):
		return "stream server is unreachable"
	case strings.Contains(lower, "timed out"):
		return "connection to the stream timed out"
	case strings.Contains(lower, "invalid data found"):
		return "no valid RTSP video found at this address"
	}
	line = strings.ReplaceAll(line, s.url, s.redacted)
	if len(line) > 200 {
		line = line[:200]
	}
	return line
}

// tailBuffer keeps only the last max bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) lastLine() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	lines := strings.Split(strings.TrimSpace(string(t.buf)), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
