import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { Event, Gate, Session, WorkItem } from "../types";
import { fmtTime, phaseLabel, relativeTime } from "../lib/format";
import { decisionSession } from "../lib/evidence";
import EvidenceHeader from "../components/EvidenceHeader";
import ValidationSummary from "../components/ValidationSummary";
import GitEvidence from "../components/GitEvidence";
import CommandEvidence from "../components/CommandEvidence";
import SessionsSection from "../components/SessionsSection";
import ProvenanceChain from "../components/ProvenanceChain";
import Timeline from "../components/Timeline";

const TERMINAL = ["complete", "failed", "blocked"];

function Selector({ items }: { items: WorkItem[] }) {
  const [q, setQ] = useState("");
  const filtered = items.filter(
    (w) =>
      w.title.toLowerCase().includes(q.toLowerCase()) ||
      w.project.toLowerCase().includes(q.toLowerCase()) ||
      w.id.toLowerCase().includes(q.toLowerCase()),
  );
  return (
    <section className="panel">
      <h2>Select a WorkItem</h2>
      <input
        className="search"
        placeholder="Filter by title, project or id…"
        value={q}
        onChange={(e) => setQ(e.target.value)}
      />
      {filtered.length === 0 ? (
        <p className="muted small">No matching WorkItems.</p>
      ) : (
        <table className="table compact">
          <thead>
            <tr>
              <th>Title</th>
              <th>Project</th>
              <th>Phase</th>
              <th>Updated</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((w) => (
              <tr key={w.id}>
                <td>
                  <a href={`#/evidence/${encodeURIComponent(w.id)}`}>{w.title}</a>
                </td>
                <td>{w.project}</td>
                <td>
                  <span className={`chip phase phase-${w.current_phase}`}>{phaseLabel(w.current_phase)}</span>
                </td>
                <td className="muted">{relativeTime(w.updated_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}

function GateEvidence({ gate, sessions }: { gate: Gate; sessions: Session[] }) {
  const decision = decisionSession(sessions);
  const plan = gate.payload?.plan;
  const hasPlan = plan && ((plan.steps?.length ?? 0) > 0);
  return (
    <section className="panel">
      <h2>Gate evidence</h2>
      <div className="row wrap gap">
        <span className={`chip ${gate.status === "approved" ? "ok" : gate.status === "pending" ? "gate" : "fail"}`}>
          {gate.status}
        </span>
        {gate.payload?.risk_assessment && (
          <span className="muted small">risk {gate.payload.risk_assessment}</span>
        )}
        {gate.resolved_at && <span className="muted small">resolved {fmtTime(gate.resolved_at)}</span>}
      </div>
      {(gate.approvals?.length ?? 0) > 0 && (
        <ul className="cmd-results">
          {gate.approvals.map((a, i) => (
            <li key={i} className="cmd-result">
              <div className="cmd-line">
                <span className={`chip ${a.decision === "approve" ? "ok" : "fail"}`}>{a.decision}</span>
                <span className="mono small">{a.user}</span>
                <span className="muted small">{fmtTime(a.at)}</span>
              </div>
              {a.comment && <p className="muted small">{a.comment}</p>}
            </li>
          ))}
        </ul>
      )}
      {hasPlan ? (
        <details className="collapse">
          <summary className="muted small">gate plan</summary>
          <pre>{JSON.stringify(plan, null, 2)}</pre>
        </details>
      ) : decision?.output?.agent_text ? (
        <details className="collapse">
          <summary className="muted small">
            <span className="tag derived">DECISION SESSION OUTPUT</span> (gate plan empty)
          </summary>
          <pre>{decision.output.agent_text}</pre>
        </details>
      ) : (
        <p className="muted small">No gate plan and no decision output available.</p>
      )}
    </section>
  );
}

export default function EvidenceCenter({ id }: { id?: string }) {
  const [items, setItems] = useState<WorkItem[]>([]);
  const [wi, setWi] = useState<WorkItem | null>(null);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [events, setEvents] = useState<Event[]>([]);
  const [gate, setGate] = useState<Gate | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.listWorkItems().then(setItems).catch((e) => setError(String(e)));
  }, []);

  useEffect(() => {
    if (!id) {
      setWi(null);
      return;
    }
    let alive = true;
    let timer: number | undefined;
    const tick = async () => {
      try {
        const item = await api.getWorkItem(id);
        const [ev, se, gt] = await Promise.all([
          api.listEvents(id),
          api.listSessions(id),
          api.getGate(id).catch(() => null),
        ]);
        if (!alive) return;
        setWi(item);
        setEvents(ev);
        setSessions(se);
        setGate(gt);
        setError(null);
        if (TERMINAL.includes(item.current_phase) && timer) {
          clearInterval(timer);
          timer = undefined;
        }
      } catch (e) {
        if (alive) setError(String(e));
      }
    };
    tick();
    timer = window.setInterval(tick, 5000);
    return () => {
      alive = false;
      if (timer) clearInterval(timer);
    };
  }, [id]);

  if (error && !wi) return <p className="error">{error}</p>;

  return (
    <div className="command-center">
      <div className="row wrap gap">
        <a href="#/" className="muted small">
          ← Mission Control
        </a>
        {id && (
          <a href="#/evidence" className="muted small">
            · choose another WorkItem
          </a>
        )}
      </div>

      {!id ? (
        <>
          <h1>Evidence Center</h1>
          <Selector items={items} />
        </>
      ) : !wi ? (
        <p className="muted">Loading evidence…</p>
      ) : (
        <>
          <EvidenceHeader wi={wi} sessions={sessions} />
          <ValidationSummary sessions={sessions} />
          <GitEvidence wi={wi} sessions={sessions} />
          <ProvenanceChain wi={wi} sessions={sessions} />
          <section className="panel">
            <h2>
              Command evidence <span className="tag persisted">PERSISTED</span>
            </h2>
            <CommandEvidence sessions={sessions} />
          </section>
          <section className="panel">
            <h2>
              Session evidence <span className="count">{sessions.length}</span>
            </h2>
            <SessionsSection sessions={sessions} />
          </section>
          {gate && <GateEvidence gate={gate} sessions={sessions} />}
          <section className="panel">
            <h2>
              Timeline / Activity <span className="count">{events.length}</span>
            </h2>
            <Timeline events={events} />
          </section>
        </>
      )}
    </div>
  );
}
