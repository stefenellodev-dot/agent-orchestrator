import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { Health } from "../types";
import { fmtTime } from "../lib/format";
import CopyableSha from "../components/CopyableSha";

const NOT_EXPOSED = [
  "uptime",
  "CPU / RAM",
  "container status",
  "PostgreSQL status",
  "OpenCode worker status",
  "deployment history",
  "metrics",
];

export default function OperationsCenter() {
  const [health, setHealth] = useState<Health | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let alive = true;
    const load = () =>
      api
        .health()
        .then((h) => alive && setHealth(h))
        .catch((e) => alive && setError(String(e)));
    load();
    const t = setInterval(load, 10000);
    return () => {
      alive = false;
      clearInterval(t);
    };
  }, []);

  if (error && !health) return <p className="error">{error}</p>;
  if (!health) return <p className="muted">Loading…</p>;

  const agents = (health.agents ?? "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);

  return (
    <div className="command-center">
      <h1>Operations</h1>

      <section className="panel">
        <h2>
          Orchestrator build / identity <span className="tag persisted">/healthz</span>
        </h2>
        <dl className="cc-identity">
          <div>
            <dt>status</dt>
            <dd>
              <span className={`chip ${health.status === "ok" ? "ok" : "fail"}`}>{health.status}</span>
            </dd>
          </div>
          <div>
            <dt>version</dt>
            <dd className="mono">{health.version}</dd>
          </div>
          <div>
            <dt>commit</dt>
            <dd>
              <CopyableSha sha={health.commit} />
            </dd>
          </div>
          <div>
            <dt>build_time</dt>
            <dd className="muted small">{fmtTime(health.build_time)}</dd>
          </div>
          <div>
            <dt>opencode</dt>
            <dd className="mono">{health.opencode ?? "—"}</dd>
          </div>
        </dl>

        <h2 className="mt">Reported agents</h2>
        {agents.length === 0 ? (
          <span className="muted small">—</span>
        ) : (
          <div className="row wrap gap">
            {agents.map((a) => (
              <span key={a} className="chip mono">
                {a}
              </span>
            ))}
          </div>
        )}
      </section>

      <section className="panel">
        <h2>Not exposed by API</h2>
        <p className="muted small">
          The backend does not expose the following; they are neither shown nor inferred.
        </p>
        <ul className="muted small not-exposed">
          {NOT_EXPOSED.map((x) => (
            <li key={x}>{x}</li>
          ))}
        </ul>
      </section>
    </div>
  );
}
