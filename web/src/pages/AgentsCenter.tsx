import { useEffect, useState } from "react";
import { loadAgentsData, type AgentsData } from "../lib/aggregate";
import { duration, fmtTime, phaseLabel } from "../lib/format";

export default function AgentsCenter() {
  const [data, setData] = useState<AgentsData | null>(null);

  useEffect(() => {
    loadAgentsData().then(setData);
  }, []);

  if (!data) return <p className="muted">Loading…</p>;

  const agents = (data.health?.agents ?? "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);

  const rank = (status: string) => (status === "running" ? 0 : status === "pending" ? 1 : 2);
  const ordered = [...data.sessions].sort(
    (a, b) => rank(a.status) - rank(b.status) || b.started_at.localeCompare(a.started_at),
  );

  return (
    <div className="command-center">
      <h1>Agents</h1>

      <section className="panel">
        <h2>
          Available agents <span className="tag persisted">/healthz</span>
        </h2>
        {agents.length === 0 ? (
          <p className="muted small">No agents reported by /healthz.</p>
        ) : (
          <div className="row wrap gap">
            {agents.map((a) => (
              <span key={a} className="chip mono">
                {a}
              </span>
            ))}
          </div>
        )}
        <p className="muted small">
          Agent names are reported by /healthz. No per-agent health is inferred; the sessions below
          are the real execution evidence.
        </p>
      </section>

      <section className="panel">
        <h2>
          Sessions <span className="count">{ordered.length}</span>
        </h2>
        {ordered.length === 0 ? (
          <p className="muted small">No sessions.</p>
        ) : (
          <table className="table compact">
            <thead>
              <tr>
                <th>Agent</th>
                <th>WorkItem</th>
                <th>Phase</th>
                <th>Status</th>
                <th>Started</th>
                <th>Finished</th>
                <th>Dur</th>
                <th>Exit</th>
                <th>Error</th>
              </tr>
            </thead>
            <tbody>
              {ordered.map((s) => {
                const active = s.status === "running" || s.status === "pending";
                return (
                  <tr key={s.id} className={active ? "row-active" : ""}>
                    <td className="mono">{s.agent || "—"}</td>
                    <td>
                      <a href={`#/workitems/${encodeURIComponent(s.workItem.id)}`}>{s.workItem.title}</a>
                    </td>
                    <td>
                      <span className={`chip phase phase-${s.phase}`}>{phaseLabel(s.phase)}</span>
                    </td>
                    <td>
                      <span
                        className={`chip ${
                          s.status === "running"
                            ? "running"
                            : s.status === "completed"
                              ? "ok"
                              : s.status === "failed"
                                ? "fail"
                                : ""
                        }`}
                      >
                        {s.status}
                      </span>
                    </td>
                    <td className="muted small">{fmtTime(s.started_at)}</td>
                    <td className="muted small">{s.completed_at ? fmtTime(s.completed_at) : "—"}</td>
                    <td className="muted small">{duration(s.started_at, s.completed_at)}</td>
                    <td>{s.exit_code}</td>
                    <td className="muted small">{s.error || "—"}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </section>

      {data.errors.length > 0 && <p className="muted small">Partial data: {data.errors.join("; ")}</p>}
    </div>
  );
}
