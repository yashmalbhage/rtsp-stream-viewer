# RTSP Stream Viewer

Add RTSP stream URLs in the browser and watch them live, several at a time, in a grid.

- **Frontend:** React + TypeScript (Vite)
- **Backend:** Go, FFmpeg, WebSockets
- **Test streams:** MediaMTX + FFmpeg, built into the backend Docker image

**Live demo:** https://rtsp-stream-viewer-1.onrender.com/

## How it works

```
Browser (React)                    Go backend                         Camera / MediaMTX
┌────────────────┐  WebSocket   ┌──────────────────────────┐   RTSP   ┌──────────────┐
│ <canvas> tile  │◄─────────────│ Stream manager           │◄─────────│ rtsp://...   │
│ one per stream │  JPEG frames │ one FFmpeg per RTSP URL, │  (TCP)   └──────────────┘
└────────────────┘  + status    │ shared by all viewers    │
                                └──────────────────────────┘
```

1. The browser opens a WebSocket to `/ws/stream?url=rtsp://...` for each tile.
2. The backend starts FFmpeg for that URL (or reuses the one already running), which decodes the RTSP video and outputs JPEG frames.
3. Each frame is sent as one binary WebSocket message and drawn on a `<canvas>`. Status changes (connecting, live, reconnecting, error) are sent as JSON text messages on the same socket.

### Design decisions

| Decision | Why |
|---|---|
| **One FFmpeg process per RTSP URL, shared by all viewers** | Ten people watching the same camera cost one decode, not ten. |
| **FFmpeg stops 10s after the last viewer leaves** | No idle processes, and a page refresh does not restart the stream. |
| **Slow viewers drop frames** | Each viewer has a one-frame mailbox where the newest frame wins, so a slow connection never adds delay or blocks other viewers. |
| **Automatic recovery** | If FFmpeg exits or no frame arrives for 15s, it is restarted with exponential backoff (1s up to 15s). The browser also reconnects if the server drops. |
| **Pause is server-side** | A paused tile tells the server to stop sending it frames, saving bandwidth. Pressing play resumes at the live position. |
| **Only `rtsp://` and `rtsps://` are accepted** | The URL is validated before it reaches FFmpeg, and passwords in URLs are masked in logs and in the UI. |
| **Stream limit (`MAX_STREAMS`)** | Protects the server from being asked to transcode an unbounded number of sources. |

### Trade-off: MJPEG over WebSockets

Frames are sent as individual JPEGs. Every frame is independent, so a new viewer sees video instantly, frame dropping is trivial, latency is low, and the browser needs no player library. The cost is bandwidth (roughly 0.5 to 1.5 Mbit/s per tile at the defaults) and no audio. For large deployments the next step would be passing H.264 through untouched (fragmented MP4 with Media Source Extensions, or WebRTC), which removes the transcode and cuts bandwidth substantially; the stream manager would stay the same.

## Run locally

### Option A: Docker (recommended, nothing else to install)

```bash
docker compose up --build
```

Open <http://localhost:5173> and click **+ Test pattern** and **+ Counter** to add the two built-in demo streams. The same streams are reachable from your machine as `rtsp://localhost:8554/demo1` and `rtsp://localhost:8554/demo2` (for example in VLC).

Note: URLs typed into the app are opened by the **backend**, so inside Docker use `rtsp://127.0.0.1:8554/demo1`, which is what the demo buttons do.

### Option B: Without Docker

Requires Go 1.23+, Node 20+, FFmpeg and [MediaMTX](https://github.com/bluenviron/mediamtx/releases).

```bash
# 1. RTSP server
mediamtx backend/docker/mediamtx.yml

# 2. Publish a test stream to it
ffmpeg -re -f lavfi -i "testsrc2=size=640x360:rate=15" \
  -c:v libx264 -preset veryfast -tune zerolatency -pix_fmt yuv420p -g 15 \
  -f rtsp -rtsp_transport tcp rtsp://127.0.0.1:8554/demo1

# 3. Backend
cd backend
go run ./cmd/server

# 4. Frontend
cd frontend
cp .env.example .env
npm install
npm run dev
```

Open <http://localhost:5173> and add `rtsp://127.0.0.1:8554/demo1`.

### What to check

| Try this | Expected |
|---|---|
| Add both demo streams | Two tiles play side by side with a green **Live** badge and an fps counter |
| Press **Pause**, then **Play** | The picture freezes, then continues from the live position |
| Add `rtsp://127.0.0.1:8554/nope` | Tile shows **Reconnecting** with "stream path not found (404)" and keeps retrying |
| Type `http://example.com` | The form rejects it before anything is sent |
| Open the app in a second tab | `curl localhost:8080/api/streams` shows `viewers: 2` for one shared stream |
| `docker compose stop backend`, then `start` | Tiles show the lost connection and recover by themselves |

Backend tests: `cd backend && go test ./...`

## Configuration

No secrets or credentials are needed. All settings are optional environment variables.

**Backend**

| Variable | Default | Meaning |
|---|---|---|
| `PORT` | `8080` | HTTP port |
| `ALLOWED_ORIGINS` | `*` | Comma-separated browser origins allowed to connect, e.g. `https://your-app.vercel.app` |
| `STREAM_FPS` | `15` | Frames per second sent to browsers |
| `STREAM_MAX_WIDTH` | `960` | Wider video is scaled down to this width |
| `STREAM_JPEG_QUALITY` | `6` | FFmpeg `-q:v`, from 2 (best) to 31 (smallest) |
| `MAX_STREAMS` | `8` | Maximum distinct RTSP sources at once |
| `IDLE_GRACE_SECONDS` | `10` | How long FFmpeg is kept after the last viewer leaves |
| `STALL_TIMEOUT_SECONDS` | `15` | Restart FFmpeg if no frame arrives for this long |
| `DEMO_STREAMS` | `true` in Docker | Start MediaMTX and the two demo streams inside the container |
| `DEMO_STREAM_URLS` | set in Docker | `Name=url,Name=url` list shown as demo buttons |
| `FFMPEG_PATH` | `ffmpeg` | Path to the FFmpeg binary |

**Frontend**

| Variable | Default | Meaning |
|---|---|---|
| `VITE_BACKEND_URL` | `http://localhost:8080` | Base URL of the backend (use `https://` in production) |

## API

| Endpoint | Description |
|---|---|
| `GET /healthz` | Health check |
| `GET /api/config` | Demo streams and limits |
| `GET /api/streams` | Active streams with state, viewer count and frame count |
| `WS /ws/stream?url=<rtsp url>` | Binary messages are JPEG frames. Text messages are `{"type":"status","state":"connecting\|live\|reconnecting","message":"..."}` or `{"type":"error","message":"..."}`. The client may send `{"type":"pause"}` and `{"type":"resume"}`. |

## Deployment

Both parts run on Render. The backend is a Docker web service because it needs FFmpeg, a long-running process and persistent WebSockets. The frontend is a static site. (The frontend also works on Vercel: set Root Directory to `frontend` and the same environment variable.)

### 1. Backend (Render Web Service)

1. Push this repository to GitHub.
2. In Render: **New > Web Service**, select the repository.
3. Set **Language** to **Docker**, **Root Directory** to `backend`, and **Instance Type** to **Free**.
4. Add the environment variables `PORT` = `8080`, `STREAM_FPS` = `10` and `STREAM_MAX_WIDTH` = `640`.
5. Deploy, then open `https://<your-backend>.onrender.com/healthz`. It should return `{"status":"ok"}`.

Alternatively, **New > Blueprint** creates the same service from `render.yaml`.

The container runs its own MediaMTX with two looping demo streams, so the live demo works without a public camera.

### 2. Frontend (Render Static Site)

1. In Render: **New > Static Site**, select the same repository.
2. Set **Root Directory** to `frontend`, **Build Command** to `npm ci && npm run build`, and **Publish Directory** to `dist`.
3. Add the environment variable `VITE_BACKEND_URL` = `https://<your-backend>.onrender.com` (no trailing slash).
4. Deploy. The variable is read at build time, so redeploy the static site if you change it.

### 3. Lock down the backend (optional)

On the backend service, set `ALLOWED_ORIGINS` to the static site URL so only your frontend can connect.

### Notes for the free tiers

- Render's free instance sleeps after 15 minutes without traffic and takes about a minute to wake. The frontend shows a "Server offline, retrying" banner and recovers by itself.
- The free instance has a fraction of a CPU, so `render.yaml` lowers the stream to 10 fps at 640px. Raise `STREAM_FPS` and `STREAM_MAX_WIDTH` on a larger instance.
- A hosted backend can only reach RTSP streams that are on the public internet, not cameras on your home network.

## Scaling further

- **More viewers per stream:** already cheap, since viewers share one FFmpeg; the limit is outgoing bandwidth.
- **More streams:** each source costs one decode and one JPEG encode. Run several backend instances behind a load balancer that routes by a hash of the RTSP URL, so viewers of the same camera land on the same instance.
- **Lower cost per stream:** pass H.264 through without transcoding, as described in the trade-off section above.

## Project structure

```
backend/
  cmd/server/main.go        entry point, graceful shutdown
  internal/config/          environment variables
  internal/stream/          FFmpeg process, frame splitting, sharing between viewers
  internal/api/             HTTP routes and the WebSocket handler
  docker/                   MediaMTX config and container entrypoint for demo streams
  Dockerfile
frontend/
  src/App.tsx               page layout, stream list, grid
  src/components/           AddStreamForm, StreamTile
  src/hooks/                useStreamSocket (WebSocket + canvas), useLocalStorage
  src/lib/api.ts            backend URLs and validation
docker-compose.yml          one-command local demo
render.yaml                 backend deployment blueprint
```
