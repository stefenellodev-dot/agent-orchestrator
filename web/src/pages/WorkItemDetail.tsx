import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { Event, Gate, Session, WorkItem } from "../types";
import ApprovalPanel from "./ApprovalPanel";
import { PhaseBadge } from "./WorkItemList";

export default function WorkItemDetail({ id }: { id: string }) {
  const [wi, setWi] = useState<WorkItem | null>(null);
  const [events, setEvents] = useState<Event[]>([]);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [gate, setGate] = useState<Gate | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function refresh() {
    try {
      const item = await api.getWorkItem(id);
      setWi(item);
      setEvents(await api.listEvents(id));
      setSessions(await api.listSessions(id));
      if (item.current_phase === "awaiting_approval") {
        setGate(await api.getGate(id));
      }
    } catch (e) {
      setError(String(e));
    }
  }

  useEffect(() => {
    refresh();
    const t = setInterval(refresh, 3000);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  if (error) return <p className="error">{error}</p>;
  if (!wi) return <p className="muted">Loading…</p>;

  return (
    <div className="detail">
      <a href="#/" className="muted">
        ← All WorkItems
      </a>
      <h1>{wi.title}</h1>
      <p className="meta">
        <PhaseBadge phase={wi.current_phase} /> · {wi.project} · {wi.id}
      </p>
      {wi.description && <p>{wi.description}</p>}

      {wi.current_phase === "awaiting_approval" && gate && (
        <ApprovalPanel workItemId={wi.id} gate={gate} onDone={refresh} />
      )}

      <h2>Validation evidence</h2>
      {sessions.filter((s) => s.phase === "validation").length === 0 && (
        <p className="muted">No validation run yet.</p>
      )}
      {sessions
        .filter((s) => s.phase === "validation")
        .map((s) => (
          <div key={s.id} className="card">
            <p className="muted">
              commit {s.output?.commit_sha?.slice(0, 12) || "n/a"} · exit {s.exit_code}
            </p>
            {s.output?.diff_stat && <pre className="diffstat">{s.output.diff_stat}</pre>}
            {(s.output?.test_commands ?? []).map((c, i) => (
              <div key={i} className={c.passed ? "cmd pass" : "cmd fail"}>
                <code>{c.command}</code> → exit {c.exit_code}
              </div>
            ))}
          </div>
        ))}

      <h2>Sessions</h2>
      <table className="table">
        <thead>
          <tr>
            <th>Phase</th>
            <th>Agent</th>
            <th>Status</th>
            <th>Exit</th>
            <th>Started</th>
          </tr>
        </thead>
        <tbody>
          {sessions.map((s) => (
            <tr key={s.id}>
              <td>{s.phase}</td>
              <td>{s.agent}</td>
              <td>{s.status}</td>
              <td>{s.exit_code}</td>
              <td className="muted">{new Date(s.started_at).toLocaleTimeString()}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <h2>Events</h2>
      <ul className="events">
        {events.map((e) => (
          <li key={e.id}>
            <span className="muted">{new Date(e.at).toLocaleTimeString()}</span> <strong>{e.type}</strong>{" "}
            <span className="muted">{e.actor}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
