package stream

import "bytes"

var (
	jpegSOI = []byte{0xFF, 0xD8} // start of image
	jpegEOI = []byte{0xFF, 0xD9} // end of image
)

// maxFrameSize bounds how much we buffer while waiting for a frame to end.
const maxFrameSize = 8 << 20

// splitJPEG is a bufio.SplitFunc that cuts ffmpeg's concatenated MJPEG output
// into individual JPEG images. Inside JPEG scan data a 0xFF byte is always
// followed by 0x00 or a restart marker, so FFD9 only ever appears as the real
// end-of-image marker.
func splitJPEG(data []byte, atEOF bool) (advance int, token []byte, err error) {
	start := bytes.Index(data, jpegSOI)
	if start < 0 {
		if atEOF {
			return len(data), nil, nil
		}
		// Keep the last byte: it may be the first half of a split marker.
		if len(data) > 0 {
			return len(data) - 1, nil, nil
		}
		return 0, nil, nil
	}
	end := bytes.Index(data[start+2:], jpegEOI)
	if end < 0 {
		if atEOF {
			return len(data), nil, nil
		}
		return start, nil, nil
	}
	stop := start + 2 + end + 2
	return stop, data[start:stop], nil
}
