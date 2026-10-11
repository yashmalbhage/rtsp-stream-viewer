import { FormEvent, useState } from "react";
import { DemoStream, validateRtspUrl } from "../lib/api";

interface Props {
  existingUrls: string[];
  demoStreams: DemoStream[];
  onAdd: (url: string, name?: string) => void;
}

export function AddStreamForm({ existingUrls, demoStreams, onAdd }: Props) {
  const [value, setValue] = useState("");
  const [error, setError] = useState<string | null>(null);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const url = value.trim();
    const problem = validateRtspUrl(url) ?? (existingUrls.includes(url) ? "This stream is already on the page." : null);
    if (problem) {
      setError(problem);
      return;
    }
    onAdd(url);
    setValue("");
    setError(null);
  };

  const availableDemos = demoStreams.filter((d) => !existingUrls.includes(d.url));

  return (
    <div className="add-stream">
      <form onSubmit={submit} noValidate>
        <label htmlFor="rtsp-url" className="sr-only">
          RTSP stream URL
        </label>
        <input
          id="rtsp-url"
          type="text"
          inputMode="url"
          autoComplete="off"
          spellCheck={false}
          placeholder="rtsp://host:554/path"
          value={value}
          onChange={(e) => {
            setValue(e.target.value);
            setError(null);
          }}
          aria-invalid={error !== null}
          aria-describedby={error ? "rtsp-url-error" : undefined}
        />
        <button type="submit" className="primary">
          Add stream
        </button>
      </form>
      {error && (
        <p id="rtsp-url-error" className="form-error" role="alert">
          {error}
        </p>
      )}
      {availableDemos.length > 0 && (
        <div className="demos">
          <span>Try a demo stream:</span>
          {availableDemos.map((demo) => (
            <button key={demo.url} type="button" className="chip" onClick={() => onAdd(demo.url, demo.name)}>
              + {demo.name}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
