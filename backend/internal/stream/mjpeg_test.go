package stream

import (
	"bufio"
	"bytes"
	"io"
	"testing"
	"testing/iotest"
)

func scanFrames(t *testing.T, r io.Reader) [][]byte {
	t.Helper()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 16), maxFrameSize)
	sc.Split(splitJPEG)
	var frames [][]byte
	for sc.Scan() {
		frames = append(frames, bytes.Clone(sc.Bytes()))
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	return frames
}

func TestSplitJPEG(t *testing.T) {
	a := []byte{0xFF, 0xD8, 1, 2, 0xFF, 0x00, 3, 0xFF, 0xD9}
	b := []byte{0xFF, 0xD8, 9, 0xFF, 0xD9}
	input := bytes.Join([][]byte{{0x00, 0x42}, a, {0x07}, b, {0xFF, 0xD8, 5}}, nil)

	readers := map[string]io.Reader{
		"whole":        bytes.NewReader(input),
		"byte by byte": iotest.OneByteReader(bytes.NewReader(input)),
	}
	for name, r := range readers {
		t.Run(name, func(t *testing.T) {
			frames := scanFrames(t, r)
			if len(frames) != 2 {
				t.Fatalf("got %d frames, want 2 (truncated trailing frame must be dropped)", len(frames))
			}
			if !bytes.Equal(frames[0], a) || !bytes.Equal(frames[1], b) {
				t.Fatalf("frames do not match input: %x %x", frames[0], frames[1])
			}
		})
	}
}
