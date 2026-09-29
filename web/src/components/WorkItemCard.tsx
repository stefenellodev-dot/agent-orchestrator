import type { EnrichedWorkItem } from "../lib/aggregate";
import { baseName, duration, phaseLabel, relativeTime, shortSha } from "../lib/format";

export default function WorkItemCard({ wi }: { wi: EnrichedWorkItem }) {
  const val = wi.lastValidation;
  const valOk = val ? val.exit_code === 0 : undefined;
  return (
    <a className="card wi-card" href={`#/workitems/${encodeURIComponent(wi.id)}`}>
      <div className="wi-card-head">
        <span className="wi-title">{wi.title}</span>
        {wi.current_phase === "awaiting_approval" && <span className="chip gate">Gate</span>}
        {(wi.current_phase === "failed" || wi.current_phase === "blocked") && (
          <span className={`chip ${wi.current_phase}`}>{phaseLabel(wi.current_phase)}</span>
        )}
      </div>
      <div className="wi-card-sub">
        <span className="chip project">{wi.project}</span>
        <span className={`chip phase phase-${wi.current_phase}`}>{phaseLabel(wi.current_phase)}</span>
      </div>
      <dl className="wi-meta">
        <div>
          <dt>branch</dt>
          <dd className="mono">{wi.base_branch}</dd>
        </div>
        <div>
          <dt>base</dt>
          <dd className="mono">{shortSha(wi.base_commit_sha)}</dd>
        </div>
        <div>
          <dt>agent</dt>
          <dd className="mono">{wi.latest?.agent ?? "—"}</dd>
        </div>
        <div>
          <dt>worktree</dt>
          <dd className="mono" title={wi.worktree_path}>
            {baseName(wi.worktree_path)}
          </dd>
        </div>
        <div>
          <dt>dur</dt>
          <dd>{duration(wi.created_at, wi.updated_at)}</dd>
        </div>
        <div>
          <dt>valid</dt>
          <dd className={valOk === undefined ? "" : valOk ? "ok-text" : "fail-text"}>
            {valOk === undefined ? "—" : valOk ? "pass" : "fail"}
          </dd>
        </div>
      </dl>
      <div className="wi-card-foot muted small">{relativeTime(wi.updated_at)}</div>
    </a>
  );
}
