import type { Session } from "../types";
import { fmtTime, phaseLabel } from "../lib/format";
import CopyableSha from "./CopyableSha";

export default function SessionsSection({ sessions }: { sessions: Session[] }) {
  const ordered = [...sessions].sort((a, b) => a.started_at.localeCompare(b.started_at));
  if (ordered.length === 0) return <p className="muted small">No sessions.</p>;

  return (
    <div className="sessions">
      {ordered.map((s) => {
        const active = s.status === "running" || s.status === "pending";
        return (
          <div key={s.id} className={`session ${active ? "active" : ""}`}>
            <div className="session-head">
              <span className={`chip phase phase-${s.phase}`}>{phaseLabel(s.phase)}</span>
              <span className="mono">{s.agent || "—"}</span>
              <span className={`chip ${active ? "running" : s.status === "completed" ? "ok" : "fail"}`}>
                {s.status}
              </span>
              <span className="muted small">exit {s.exit_code}</span>
              <span className="muted small mono session-id">{s.id}</span>
            </div>
            <div className="session-meta muted small">
              started {fmtTime(s.started_at)}
              {s.completed_at ? ` · finished ${fmtTime(s.completed_at)}` : ""}
              {s.output?.commit_sha ? (
                <>
                  {" · "}
                  <CopyableSha sha={s.output.commit_sha} label="commit" />
                </>
              ) : null}
            </div>
            {s.output?.diff_stat && <pre className="diffstat">{s.output.diff_stat}</pre>}
            {s.error && <p className="error small">{s.error}</p>}
            {s.output?.agent_text && (
              <details className="collapse">
                <summary className="muted small">agent output</summary>
                <pre>{s.output.agent_text}</pre>
              </details>
            )}
          </div>
        );
      })}
    </div>
  );
}
