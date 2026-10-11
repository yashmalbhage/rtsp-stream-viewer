#!/bin/sh
# Starts the optional demo RTSP streams (MediaMTX + looping test videos), then
# the Go server. Set DEMO_STREAMS=false to run the server on its own.
set -e

publish() {
  while true; do
    ffmpeg -hide_banner -loglevel error -nostdin -re -stream_loop -1 -i "$1" \
      -c copy -f rtsp -rtsp_transport tcp "rtsp://127.0.0.1:8554/$2" || true
    sleep 2
  done
}

if [ "$DEMO_STREAMS" = "true" ]; then
  mediamtx /etc/mediamtx.yml &
  publish /demo/demo1.mp4 demo1 &
  publish /demo/demo2.mp4 demo2 &
fi

exec server
