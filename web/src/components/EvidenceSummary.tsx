import type { Session, WorkItem } from "../types";
import CopyableSha from "./CopyableSha";

function latest(sessions: Session[], phase: string): Session | undefined {
  return sessions
    .filter((s) => s.phase === phase)
    .sort((a, b) => b.started_at.localeCompare(a.started_at))[0];
}

export default function EvidenceSummary({ wi, sessions }: { wi: WorkItem; sessions: Session[] }) {
  const impl = latest(sessions, "implementation");
  const val = latest(sessions, "validation");
  const commit = impl?.output?.commit_sha || val?.output?.commit_sha;
  const valCmds = val
    ? [...(val.output?.test_commands ?? []), ...(val.output?.lint_commands ?? []), ...(val.output?.typecheck_commands ?? [])]
    : [];
  const diffLine = impl?.output?.diff_stat?.trim().split("\n").pop()?.trim();

  return (
    <dl className="evidence-grid">
      <div>
        <dt>base SHA</dt>
        <dd>
          <CopyableSha sha={wi.base_commit_sha} />
        </dd>
      </div>
      <div>
        <dt>implementation commit</dt>
        <dd>
          <CopyableSha sha={commit} />
        </dd>
      </div>
      <div>
        <dt>diff</dt>
        <dd className="mono small">{diffLine || "—"}</dd>
      </div>
      <div>
        <dt>validation</dt>
        <dd className={val ? (val.exit_code === 0 ? "ok-text" : "fail-text") : "muted"}>
          {val ? (val.exit_code === 0 ? "pass" : "fail") : "not run"} ({valCmds.length} cmd)
        </dd>
      </div>
      <div>
        <dt>sessions</dt>
        <dd>{sessions.length}</dd>
      </div>
    </dl>
  );
}
