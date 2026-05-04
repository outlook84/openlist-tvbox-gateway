import { useEffect, useRef, useState } from "react";
import { ListRestart, Pause, Play } from "lucide-react";
import { getLogs } from "../api";
import type { LogEntry } from "../types";
import type { T } from "../shared";
import { localizeError } from "../errors";
import { Field, Status } from "../components/ui";

type LogLevelFilter = "debug" | "info" | "warn" | "error";

export function LogsPanel({ t }: { t: T }) {
  const [entries, setEntries] = useState<LogEntry[]>([]);
  const [level, setLevel] = useState<LogLevelFilter>("debug");
  const [paused, setPaused] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  const [error, setError] = useState("");
  const listRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    let cancelled = false;
    setError("");
    getLogs(300, level)
      .then((result) => {
        if (!cancelled) {
          setEntries(result.logs || []);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setError(localizeError(err, t));
        }
      });
    return () => {
      cancelled = true;
    };
  }, [level, t]);

  useEffect(() => {
    if (paused) {
      return;
    }
    const params = new URLSearchParams({ level });
    const source = new EventSource(`/admin/logs/stream?${params.toString()}`);
    source.addEventListener("log", (event) => {
      try {
        const entry = JSON.parse((event as MessageEvent).data) as LogEntry;
        setEntries((current) => [...current.slice(-499), entry]);
      } catch {
        // Ignore malformed stream events; the initial snapshot remains usable.
      }
    });
    source.onerror = () => setError(t("reconnectingLogs"));
    source.onopen = () => setError("");
    return () => source.close();
  }, [level, paused, t]);

  useEffect(() => {
    if (!autoScroll || !listRef.current) {
      return;
    }
    listRef.current.scrollTop = listRef.current.scrollHeight;
  }, [entries, autoScroll]);

  return (
    <section className="panel logs-panel">
      <div className="logs-toolbar">
        <div className="logs-title">
          <h2>{t("liveLogs")}</h2>
          <p className="muted">{paused ? t("logStreamPaused") : t("recentLogs")}</p>
        </div>
        <div className="logs-actions">
          <Field label={t("logLevel")}>
            <select value={level} onChange={(event) => setLevel(event.target.value as LogLevelFilter)}>
              <option value="debug">{t("allLevels")}</option>
              <option value="info">INFO</option>
              <option value="warn">WARN</option>
              <option value="error">ERROR</option>
            </select>
          </Field>
          <label className="check-row logs-check">
            <input type="checkbox" checked={autoScroll} onChange={(event) => setAutoScroll(event.target.checked)} />
            <span>{t("autoScroll")}</span>
          </label>
          <button type="button" className="small" onClick={() => setPaused((current) => !current)}>
            {paused ? <Play size={16} /> : <Pause size={16} />} {paused ? t("resume") : t("pause")}
          </button>
          <button type="button" className="small" onClick={() => setEntries([])}>
            <ListRestart size={16} /> {t("clearView")}
          </button>
        </div>
      </div>
      {error && <Status message="" error={error} />}
      <div className="log-list" ref={listRef}>
        {entries.length === 0 ? (
          <p className="empty-state">{t("noLogs")}</p>
        ) : (
          entries.map((entry, index) => <LogRow entry={entry} key={`${entry.time}-${index}`} />)
        )}
      </div>
    </section>
  );
}

export function LogRow({ entry }: { entry: LogEntry }) {
  const attrs = entry.attrs && Object.keys(entry.attrs).length > 0 ? JSON.stringify(entry.attrs) : "";
  return (
    <article className={`log-row level-${entry.level.toLowerCase()}`}>
      <time dateTime={entry.time}>{formatLogTime(entry.time)}</time>
      <span className="log-level">{entry.level}</span>
      <span className="log-message">{entry.message}</span>
      {attrs && <code>{attrs}</code>}
    </article>
  );
}

export function formatLogTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleTimeString(undefined, { hour12: false });
}
