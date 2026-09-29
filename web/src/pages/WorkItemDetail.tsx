import { useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type { Event, Gate, Session, WorkItem } from "../types";
import { fmtTime, relativeTime } from "../lib/format";
import ApprovalPanel from "./ApprovalPanel";
import PhaseBadge from "../components/PhaseBadge";
import CopyableSha from "../components/CopyableSha";
import StateFlow from "../components/StateFlow";
import Timeline from "../components/Timeline";
import SessionsSection from "../components/SessionsSection";
import ValidationSection from "../components/ValidationSection";
import EvidenceSummary from "../components/EvidenceSummary";

const TERMINAL = ["complete", "failed", "blocked"];

export default function WorkItemDetail({ id }: { id: string }) {
  const [wi, setWi] = useState<WorkItem | null>(null);
  const [events, setEvents] = useState<Event[]>([]);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [gate, setGate] = useState<Gate | null>(null);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    const item = await api.getWorkItem(id);
    setWi(item);
    const [ev, se, gt] = await Promise.all([
      api.listEvents(id),
      api.listSessions(id),
      api.getGate(id).catch(() => null),
    ]);
    setEvents(ev);
    setSessions(se);
    setGate(gt);
  }, [id]);

  const terminal = wi ? TERMINAL.includes(wi.current_phase) : false;

  useEffect(() => {
    let alive = true;
    const tick = async () => {
      try {
        await refresh();
        if (alive) setError(null);
      } catch (e) {
        if (alive) setError(String(e));
      }
    };
    tick();
    if (terminal) return () => { alive = false; };
    const t = setInterval(tick, 4000);
    return () => {
      alive = false;
      clearInterval(t);
    };
  }, [refresh, terminal]);

  if (error && !wi) return <p className="error">{error}</p>;
  if (!wi) return <p className="muted">Loading…</p>;

  const decision = sessions
    .filter((s) => s.phase === "decision")
    .sort((a, b) => b.started_at.localeCompare(a.started_at))[0];
  const isGate = wi.current_phase === "awaiting_approval";

  return (
    <div className="command-center">
      <a href="#/" className="muted small">
        ← Mission Control
      </a>

      {/* 1. Header / Identity */}
      <header className="cc-header panel">
        <div className="cc-title">
          <h1>{wi.title}</h1>
          <div className="row wrap gap">
            <PhaseBadge phase={wi.current_phase} />
            <span className="chip project">{wi.project}</span>
            {wi.priority && <span className="chip">{wi.priority}</span>}
            <span className="muted small mono" title={wi.id}>
              {wi.id}
            </span>
          </div>
        </div>
        <dl className="cc-identity">
          <div>
            <dt>branch</dt>
            <dd className="mono">{wi.base_branch}</dd>
          </div>
          <div>
            <dt>base SHA</dt>
            <dd>
              <CopyableSha sha={wi.base_commit_sha} />
            </dd>
          </div>
          <div>
            <dt>worktree</dt>
            <dd className="mono small" title={wi.worktree_path}>
              {wi.worktree_path || "—"}
            </dd>
          </div>
          <div>
            <dt>created</dt>
            <dd className="muted small">{fmtTime(wi.created_at)}</dd>
          </div>
          <div>
            <dt>updated</dt>
            <dd className="muted small">
              {relativeTime(wi.updated_at)} · {fmtTime(wi.updated_at)}
            </dd>
          </div>
        </dl>
      </header>

      {/* 2. Objective */}
      <section className="panel">
        <h2>Objective</h2>
        <p className="cc-objective">{wi.description || <span className="muted">No description.</span>}</p>
        {wi.metadata && Object.keys(wi.metadata).length > 0 && (
          <details className="collapse">
            <summary className="muted small">metadata</summary>
            <pre>{JSON.stringify(wi.metadata, null, 2)}</pre>
          </details>
        )}
      </section>

      {/* 3. Current State */}
      <section className="panel">
        <h2>Current State</h2>
        <StateFlow current={wi.current_phase} />
        <div className="row wrap gap gate-compact">
          <span className="muted small">Gate:</span>
          <span className={`chip ${gate ? (gate.status === "approved" ? "ok" : gate.status === "pending" ? "gate" : "fail") : ""}`}>
            {gate ? gate.status : isGate ? "pending" : "—"}
          </span>
          {gate?.resolved_at && <span className="muted small">resolved {fmtTime(gate.resolved_at)}</span>}
        </div>
      </section>

      {/* 4. Human Gate (priority when awaiting) */}
      {isGate && gate && (
        <section className="cc-gate">
          <ApprovalPanel workItemId={wi.id} gate={gate} onDone={refresh} />
          {decision?.output?.agent_text && (
            <div className="panel">
              <h2>Proposed plan (Decision)</h2>
              <details className="collapse" open>
                <summary className="muted small">decision output</summary>
                <pre>{decision.output.agent_text}</pre>
              </details>
            </div>
          )}
        </section>
      )}

      {/* 5. Evidence summary */}
      <section className="panel">
        <h2>Evidence summary</h2>
        <EvidenceSummary wi={wi} sessions={sessions} />
        <p>
          <a href={`#/evidence/${encodeURIComponent(wi.id)}`}>Open Evidence Center →</a>
        </p>
      </section>

      {/* 6. Validation */}
      <section className="panel">
        <h2>Validation</h2>
        <ValidationSection sessions={sessions} />
      </section>

      {/* 7. Agent / Session */}
      <section className="panel">
        <h2>
          Agent / Sessions <span className="count">{sessions.length}</span>
        </h2>
        <SessionsSection sessions={sessions} />
      </section>

      {/* 8. Timeline / Activity */}
      <section className="panel">
        <h2>
          Timeline / Activity <span className="count">{events.length}</span>
        </h2>
        <Timeline events={events} />
      </section>
    </div>
  );
}
