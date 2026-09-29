import type { Session, WorkItem } from "../types";
import CopyableSha from "./CopyableSha";
import { implementationSession } from "../lib/evidence";

export default function GitEvidence({ wi, sessions }: { wi: WorkItem; sessions: Session[] }) {
  const impl = implementationSession(sessions);
  const diff = impl?.output?.diff;
  const diffStat = impl?.output?.diff_stat;
  const commit = impl?.output?.commit_sha;

  return (
    <section className="panel">
      <h2>
        Git evidence <span className="tag persisted">PERSISTED</span>
      </h2>
      <dl className="evidence-grid">
        <div>
          <dt>base SHA</dt>
          <dd>
            <CopyableSha sha={wi.base_commit_sha} />
          </dd>
        </div>
        <div>
          <dt>
            implementation commit <span className="tag derived">DERIVED</span>
          </dt>
          <dd>
            <CopyableSha sha={commit} />
          </dd>
        </div>
      </dl>

      {diffStat ? (
        <pre className="diffstat">{diffStat}</pre>
      ) : (
        <p className="muted small">No diff stat persisted.</p>
      )}

      {diff ? (
        <details className="collapse">
          <summary className="muted small">
            show full diff ({Math.round(diff.length / 1024)} KB)
          </summary>
          <pre className="diff">{diff}</pre>
        </details>
      ) : (
        <p className="muted small">No diff persisted in session output.</p>
      )}

      <p className="muted small">
        Working tree status: not available in persisted evidence (not claimed).
      </p>
    </section>
  );
}
