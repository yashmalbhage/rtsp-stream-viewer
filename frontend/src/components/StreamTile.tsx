import { useRef, useState } from "react";
import { useStreamSocket } from "../hooks/useStreamSocket";
import { displayUrl } from "../lib/api";

export interface StreamItem {
  id: string;
  url: string;
  name?: string;
}

interface Props {
  stream: StreamItem;
  onRemove: (id: string) => void;
}

const STATE_LABEL = {
  connecting: "Connecting",
  live: "Live",
  reconnecting: "Reconnecting",
  error: "Error",
} as const;

export function StreamTile({ stream, onRemove }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const tileRef = useRef<HTMLElement>(null);
  const [paused, setPaused] = useState(false);
  const { status, fps, hasFrame, retry } = useStreamSocket(stream.url, canvasRef, paused);

  const badge = paused ? "paused" : status.state;
  const label = paused ? "Paused" : STATE_LABEL[status.state];
  const showOverlay = !paused && status.state !== "live";
  const title = stream.name ?? displayUrl(stream.url);

  const toggleFullscreen = () => {
    if (document.fullscreenElement) void document.exitFullscreen();
    else void tileRef.current?.requestFullscreen?.();
  };

  return (
    <article className="tile" ref={tileRef}>
      <header className="tile-header">
        <span className={`badge badge-${badge}`}>{label}</span>
        <span className="tile-title" title={displayUrl(stream.url)}>
          {title}
        </span>
        {status.state === "live" && !paused && <span className="fps">{fps} fps</span>}
      </header>

      <div className="tile-video">
        <canvas ref={canvasRef} className={hasFrame ? "" : "empty"} aria-label={`Live video from ${title}`} />
        {showOverlay && (
          <div className={`overlay overlay-${status.state}`} role="status">
            {status.state !== "error" && <div className="spinner" aria-hidden="true" />}
            <strong>{status.state === "connecting" ? "Connecting to stream…" : label}</strong>
            {status.message && <p>{status.message}</p>}
            {status.state === "error" && (
              <button type="button" onClick={retry}>
                Try again
              </button>
            )}
          </div>
        )}
      </div>

      <footer className="tile-controls">
        <button
          type="button"
          onClick={() => setPaused((p) => !p)}
          disabled={status.state === "error"}
          aria-pressed={paused}
        >
          {paused ? "▶ Play" : "⏸ Pause"}
        </button>
        <button type="button" onClick={toggleFullscreen}>
          ⛶ Fullscreen
        </button>
        <button type="button" className="danger" onClick={() => onRemove(stream.id)}>
          ✕ Remove
        </button>
      </footer>
    </article>
  );
}
