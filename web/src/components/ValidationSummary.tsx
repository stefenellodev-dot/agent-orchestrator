import type { Session } from "../types";
import { validationOutcome } from "../lib/evidence";

export default function ValidationSummary({ sessions }: { sessions: Session[] }) {
  const v = validationOutcome(sessions);
  const tone = v.state === "PASS" ? "ok" : v.state === "FAIL" ? "fail" : "";
  return (
    <section className="panel">
      <h2>
        Validation summary <span className="tag persisted">PERSISTED</span>
      </h2>
      <div className="row wrap gap">
        <span className={`chip ${tone}`}>{v.state}</span>
        <span className="muted small">
          {v.checks} checks · {v.passed} passed · {v.failed} failed
        </span>
        {v.checks > 0 && <span className="muted small">· {v.totalMs}ms total</span>}
      </div>
      {v.session && (
        <div className="muted small">
          validation session {v.session.id.slice(0, 10)} · agent {v.session.agent} · exit {v.session.exit_code}
        </div>
      )}
      {v.state === "NOT RUN" && <p className="muted small">No validation session recorded for this WorkItem.</p>}
    </section>
  );
}
