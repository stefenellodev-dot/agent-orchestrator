import type { Session, WorkItem } from "../types";
import { commandsOf, implementationSession, validationSession } from "../lib/evidence";

interface Node {
  label: string;
  detail: string;
  kind: "persisted" | "derived";
}

export default function ProvenanceChain({ wi, sessions }: { wi: WorkItem; sessions: Session[] }) {
  const impl = implementationSession(sessions);
  const val = validationSession(sessions);
  const cmdCount = sessions.reduce((n, s) => n + commandsOf(s).length, 0);

  const nodes: Node[] = [
    { label: "WorkItem", detail: wi.id.slice(0, 10), kind: "persisted" },
    { label: "Sessions", detail: String(sessions.length), kind: "persisted" },
    { label: "Commands", detail: String(cmdCount), kind: "persisted" },
    { label: "Commit/Diff", detail: impl?.output?.commit_sha?.slice(0, 10) ?? "—", kind: "derived" },
    { label: "Validation", detail: val ? (val.exit_code === 0 ? "PASS" : "FAIL") : "NOT RUN", kind: "persisted" },
  ];

  return (
    <section className="panel">
      <h2>Timeline / Provenance</h2>
      <div className="prov">
        {nodes.map((n, i) => (
          <span key={n.label} className="prov-node">
            <span className="prov-box">
              <span className="prov-label">{n.label}</span>
              <span className="prov-detail mono small">{n.detail}</span>
              <span className={`tag ${n.kind}`}>{n.kind.toUpperCase()}</span>
            </span>
            {i < nodes.length - 1 && <span className="prov-arrow">→</span>}
          </span>
        ))}
      </div>
      <p className="muted small">
        Built from persisted sessions/events. The Commit/Diff → session association is <strong>derived</strong>{" "}
        (there is no first-class link); commands belong to their session.
      </p>
    </section>
  );
}
