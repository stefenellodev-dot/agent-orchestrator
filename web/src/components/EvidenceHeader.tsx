import type { Session, WorkItem } from "../types";
import { fmtTime } from "../lib/format";
import CopyableSha from "./CopyableSha";
import PhaseBadge from "./PhaseBadge";
import { implementationCommit } from "../lib/evidence";

export default function EvidenceHeader({ wi, sessions }: { wi: WorkItem; sessions: Session[] }) {
  const implSha = implementationCommit(sessions);
  return (
    <header className="panel ev-header">
      <div className="cc-title">
        <h1>{wi.title}</h1>
        <div className="row wrap gap">
          <PhaseBadge phase={wi.current_phase} />
          <span className="chip project">{wi.project}</span>
          <span className="muted small mono">{wi.id}</span>
        </div>
      </div>
      <dl className="cc-identity">
        <div>
          <dt>branch</dt>
          <dd className="mono">{wi.base_branch}</dd>
        </div>
        <div>
          <dt>
            base SHA <span className="tag persisted">PERSISTED</span>
          </dt>
          <dd>
            <CopyableSha sha={wi.base_commit_sha} />
          </dd>
        </div>
        <div>
          <dt>
            implementation SHA <span className="tag derived">DERIVED</span>
          </dt>
          <dd>
            <CopyableSha sha={implSha} />{" "}
            <span className="muted small">from implementation session</span>
          </dd>
        </div>
        <div>
          <dt>created</dt>
          <dd className="muted small">{fmtTime(wi.created_at)}</dd>
        </div>
        <div>
          <dt>updated</dt>
          <dd className="muted small">{fmtTime(wi.updated_at)}</dd>
        </div>
      </dl>
    </header>
  );
}
