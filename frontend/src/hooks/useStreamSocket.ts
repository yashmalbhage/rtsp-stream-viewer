import { RefObject, useCallback, useEffect, useRef, useState } from "react";
import { streamSocketUrl } from "../lib/api";

export type StreamState = "connecting" | "live" | "reconnecting" | "error";

export interface StreamStatus {
  state: StreamState;
  message?: string;
}

const CLOSE_REJECTED = 4000; // must match the backend
const MAX_RETRY_DELAY_MS = 10_000;

/**
 * Opens a WebSocket for one RTSP URL and paints the JPEG frames it receives
 * onto the given canvas. Reconnects automatically if the server drops.
 */
export function useStreamSocket(
  rtspUrl: string,
  canvasRef: RefObject<HTMLCanvasElement>,
  paused: boolean,
) {
  const [status, setStatus] = useState<StreamStatus>({ state: "connecting" });
  const [fps, setFps] = useState(0);
  const [hasFrame, setHasFrame] = useState(false);
  const [generation, setGeneration] = useState(0);
  const socketRef = useRef<WebSocket | null>(null);
  const pausedRef = useRef(paused);

  // Tell the server to stop sending frames while paused, to save bandwidth.
  useEffect(() => {
    pausedRef.current = paused;
    const ws = socketRef.current;
    if (ws?.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: paused ? "pause" : "resume" }));
    }
  }, [paused]);

  useEffect(() => {
    let disposed = false;
    let rejected = false;
    let retries = 0;
    let retryTimer: number | undefined;
    let ws: WebSocket | null = null;
    let decoding = false;
    let pending: ArrayBuffer | null = null;
    let frameCount = 0;

    // Decode one frame at a time; if frames arrive faster than they can be
    // drawn, only the newest one is kept.
    const draw = async (data: ArrayBuffer) => {
      if (decoding) {
        pending = data;
        return;
      }
      decoding = true;
      try {
        const bitmap = await createImageBitmap(new Blob([data], { type: "image/jpeg" }));
        const canvas = canvasRef.current;
        if (canvas && !disposed && !pausedRef.current) {
          if (canvas.width !== bitmap.width || canvas.height !== bitmap.height) {
            canvas.width = bitmap.width;
            canvas.height = bitmap.height;
          }
          canvas.getContext("2d")?.drawImage(bitmap, 0, 0);
          frameCount++;
          setHasFrame(true);
        }
        bitmap.close();
      } catch {
        // A corrupt frame is skipped; the next one replaces it.
      } finally {
        decoding = false;
        if (pending && !disposed) {
          const next = pending;
          pending = null;
          void draw(next);
        }
      }
    };

    const connect = () => {
      ws = new WebSocket(streamSocketUrl(rtspUrl));
      ws.binaryType = "arraybuffer";
      socketRef.current = ws;

      ws.onopen = () => {
        retries = 0;
        if (pausedRef.current) ws?.send(JSON.stringify({ type: "pause" }));
      };

      ws.onmessage = (event) => {
        if (typeof event.data !== "string") {
          if (!pausedRef.current) void draw(event.data as ArrayBuffer);
          return;
        }
        try {
          const msg = JSON.parse(event.data);
          if (msg.type === "error") {
            rejected = true;
            setStatus({ state: "error", message: msg.message });
          } else if (msg.type === "status") {
            setStatus({ state: msg.state, message: msg.message });
          }
        } catch {
          // Ignore malformed control messages.
        }
      };

      ws.onclose = (event) => {
        if (disposed) return;
        if (rejected || event.code === CLOSE_REJECTED) {
          setStatus((s) => (s.state === "error" ? s : { state: "error", message: "Request rejected by the server." }));
          return;
        }
        const delay = Math.min(1000 * 2 ** retries, MAX_RETRY_DELAY_MS);
        retries++;
        setStatus({
          state: "reconnecting",
          message: `Lost connection to the server. Retrying in ${Math.round(delay / 1000)}s.`,
        });
        retryTimer = window.setTimeout(connect, delay);
      };
    };

    setStatus({ state: "connecting" });
    setHasFrame(false);
    connect();
    const fpsTimer = window.setInterval(() => {
      setFps(frameCount);
      frameCount = 0;
    }, 1000);

    return () => {
      disposed = true;
      window.clearTimeout(retryTimer);
      window.clearInterval(fpsTimer);
      socketRef.current = null;
      ws?.close();
    };
  }, [rtspUrl, canvasRef, generation]);

  const retry = useCallback(() => setGeneration((g) => g + 1), []);

  return { status, fps, hasFrame, retry };
}
