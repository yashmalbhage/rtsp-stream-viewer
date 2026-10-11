# --- build the Go server ---
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# --- MediaMTX binary for the demo RTSP streams ---
FROM bluenviron/mediamtx:1.9.3 AS mediamtx

# --- runtime ---
FROM alpine:3.20
RUN apk add --no-cache ffmpeg ca-certificates

# Pre-encode two short test videos at build time so that publishing them as
# RTSP at runtime is a cheap stream copy (important on small free instances).
RUN mkdir /demo \
 && ffmpeg -hide_banner -loglevel error -f lavfi -i "testsrc2=size=640x360:rate=15" -t 60 \
      -c:v libx264 -preset veryfast -tune zerolatency -pix_fmt yuv420p -g 15 -bf 0 /demo/demo1.mp4 \
 && ffmpeg -hide_banner -loglevel error -f lavfi -i "testsrc=size=640x360:rate=15" -t 60 \
      -c:v libx264 -preset veryfast -tune zerolatency -pix_fmt yuv420p -g 15 -bf 0 /demo/demo2.mp4

COPY --from=mediamtx /mediamtx /usr/local/bin/mediamtx
COPY --from=build /out/server /usr/local/bin/server
COPY docker/mediamtx.yml /etc/mediamtx.yml
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENV PORT=8080 \
    DEMO_STREAMS=true \
    DEMO_STREAM_URLS="Test pattern=rtsp://127.0.0.1:8554/demo1,Counter=rtsp://127.0.0.1:8554/demo2"

EXPOSE 8080 8554
ENTRYPOINT ["/entrypoint.sh"]
