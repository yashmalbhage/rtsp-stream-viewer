export const BACKEND_URL = (
  import.meta.env.VITE_BACKEND_URL ?? "http://localhost:8080"
).replace(/\/+$/, "");

export interface DemoStream {
  name: string;
  url: string;
}

export interface ServerConfig {
  demoStreams: DemoStream[];
  maxStreams: number;
  fps: number;
}

export async function fetchConfig(signal?: AbortSignal): Promise<ServerConfig> {
  const res = await fetch(`${BACKEND_URL}/api/config`, { signal });
  if (!res.ok) throw new Error(`Backend responded with ${res.status}`);
  return res.json();
}

export function streamSocketUrl(rtspUrl: string): string {
  const base = BACKEND_URL.replace(/^http/, "ws");
  return `${base}/ws/stream?url=${encodeURIComponent(rtspUrl)}`;
}

/** Returns an error message, or null when the URL looks like a valid RTSP URL. */
export function validateRtspUrl(value: string): string | null {
  const trimmed = value.trim();
  if (!trimmed) return "Enter an RTSP URL.";
  if (!/^rtsps?:\/\//i.test(trimmed)) return "The URL must start with rtsp:// or rtsps://";
  try {
    if (!new URL(trimmed).hostname) return "The URL is missing a host.";
  } catch {
    return "This is not a valid URL.";
  }
  return null;
}

/** Hides any username and password before a URL is shown on screen. */
export function displayUrl(rtspUrl: string): string {
  try {
    const u = new URL(rtspUrl);
    return `${u.host}${u.pathname}${u.search}`;
  } catch {
    return rtspUrl;
  }
}
