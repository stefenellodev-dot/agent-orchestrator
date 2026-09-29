import type { SessionWithItem } from "../lib/aggregate";
import { duration, phaseLabel } from "../lib/format";

export default function SessionsPanel({ sessions }: { sessions: SessionWithItem[] }) {
  const active = sessions.filter((s) => s.status === "running" || s.status === "pending");
  const recent = [...sessions].sort((a, b) => b.started_at.localeCompare(a.started_at)).slice(0, 12);
  const list = active.length > 0 ? active : recent;

  return (
    <section className="panel">
      <h2>
        {active.length > 0 ? "Active sessions" : "Recent sessions"} <span className="count">{list.length}</span>
      </h2>
      {list.length === 0 ? (
        <p className="muted small">No sessions yet.</p>
      ) : (
        <table className="table compact">
          <thead>
            <tr>
              <th>Agent</th>
              <th>WorkItem</th>
              <th>Phase</th>
              <th>Status</th>
              <th>Duration</th>
            </tr>
          </thead>
          <tbody>
            {list.map((s) => (
              <tr key={s.id}>
                <td className="mono">{s.agent}</td>
                <td>
                  <a href={`#/workitems/${encodeURIComponent(s.workItem.id)}`}>{s.workItem.title}</a>
                </td>
                <td>
                  <span className={`chip phase phase-${s.phase}`}>{phaseLabel(s.phase)}</span>
                </td>
                <td>{s.status}</td>
                <td className="muted">{duration(s.started_at, s.completed_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
