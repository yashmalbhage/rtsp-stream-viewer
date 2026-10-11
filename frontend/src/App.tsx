import { useEffect, useState } from "react";
import { AddStreamForm } from "./components/AddStreamForm";
import { StreamItem, StreamTile } from "./components/StreamTile";
import { useLocalStorage } from "./hooks/useLocalStorage";
import { BACKEND_URL, DemoStream, fetchConfig } from "./lib/api";

type Columns = "auto" | "1" | "2" | "3";
type BackendState = "checking" | "online" | "offline";

const COLUMN_OPTIONS: { value: Columns; label: string }[] = [
  { value: "auto", label: "Auto" },
  { value: "1", label: "1" },
  { value: "2", label: "2" },
  { value: "3", label: "3" },
];

export default function App() {
  const [streams, setStreams] = useLocalStorage<StreamItem[]>("rtsp-viewer.streams", []);
  const [columns, setColumns] = useLocalStorage<Columns>("rtsp-viewer.columns", "auto");
  const [demoStreams, setDemoStreams] = useState<DemoStream[]>([]);
  const [backend, setBackend] = useState<BackendState>("checking");

  // Poll until the backend answers: free hosting tiers sleep when idle and
  // can take up to a minute to wake.
  useEffect(() => {
    const controller = new AbortController();
    let timer: number | undefined;
    const load = async () => {
      try {
        const config = await fetchConfig(controller.signal);
        setDemoStreams(config.demoStreams ?? []);
        setBackend("online");
      } catch {
        if (controller.signal.aborted) return;
        setBackend("offline");
        timer = window.setTimeout(load, 5000);
      }
    };
    void load();
    return () => {
      controller.abort();
      window.clearTimeout(timer);
    };
  }, []);

  const addStream = (url: string, name?: string) =>
    setStreams((list) => [...list, { id: crypto.randomUUID(), url, name }]);

  const removeStream = (id: string) => setStreams((list) => list.filter((s) => s.id !== id));

  return (
    <div className="app">
      <header className="app-header">
        <div>
          <h1>RTSP Stream Viewer</h1>
          <p>Add RTSP camera URLs and watch them live, side by side.</p>
        </div>
        <span className={`backend backend-${backend}`}>
          {backend === "online" ? "Server online" : backend === "checking" ? "Checking server…" : "Server offline"}
        </span>
      </header>

      {backend === "offline" && (
        <div className="banner" role="alert">
          Cannot reach the streaming server at <code>{BACKEND_URL}</code>. If it is hosted on a free plan it may be
          waking up, which can take about a minute. Retrying automatically…
        </div>
      )}

      <AddStreamForm existingUrls={streams.map((s) => s.url)} demoStreams={demoStreams} onAdd={addStream} />

      {streams.length > 0 && (
        <div className="toolbar">
          <span>
            {streams.length} {streams.length === 1 ? "stream" : "streams"}
          </span>
          <div className="segmented" role="group" aria-label="Grid columns">
            <span>Columns</span>
            {COLUMN_OPTIONS.map((option) => (
              <button
                key={option.value}
                type="button"
                className={columns === option.value ? "active" : ""}
                aria-pressed={columns === option.value}
                onClick={() => setColumns(option.value)}
              >
                {option.label}
              </button>
            ))}
          </div>
          <button type="button" className="danger" onClick={() => setStreams([])}>
            Remove all
          </button>
        </div>
      )}

      {streams.length === 0 ? (
        <div className="empty-state">
          <h2>No streams yet</h2>
          <p>Paste an RTSP URL above{demoStreams.length > 0 ? " or pick a demo stream" : ""} to start watching.</p>
        </div>
      ) : (
        <main className={`grid grid-${columns}`}>
          {streams.map((stream) => (
            <StreamTile key={stream.id} stream={stream} onRemove={removeStream} />
          ))}
        </main>
      )}
    </div>
  );
}
