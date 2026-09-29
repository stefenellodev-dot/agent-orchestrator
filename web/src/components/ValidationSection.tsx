import type { Session } from "../types";
import { fmtTime } from "../lib/format";
import CopyableSha from "./CopyableSha";

export default function ValidationSection({ sessions }: { sessions: Session[] }) {
  const vals = sessions
    .filter((s) => s.phase === "validation")
    .sort((a, b) => b.started_at.localeCompare(a.started_at));
  if (vals.length === 0) return <p className="muted small">Validation not run.</p>;

  const s = vals[0];
  const cmds = [
    ...(s.output?.test_commands ?? []),
    ...(s.output?.lint_commands ?? []),
    ...(s.output?.typecheck_commands ?? []),
  ];

  return (
    <div className="validation">
      <div className="row wrap gap">
        <span className={`chip ${s.exit_code === 0 ? "ok" : "fail"}`}>
          {s.exit_code === 0 ? "PASS" : "FAIL"} · exit {s.exit_code}
        </span>
        <span className="muted small">{fmtTime(s.completed_at ?? s.started_at)}</span>
        {s.output?.commit_sha && <CopyableSha sha={s.output.commit_sha} label="commit" />}
      </div>
      {cmds.length === 0 ? (
        <p className="muted small">No command results recorded.</p>
      ) : (
        <ul className="cmd-results">
          {cmds.map((c, i) => (
            <li key={i} className={`cmd-result ${c.passed ? "pass" : "fail"}`}>
              <div className="cmd-line">
                <span className={`chip ${c.passed ? "ok" : "fail"}`}>{c.passed ? "pass" : "fail"}</span>
                <code>{c.command}</code>
                <span className="muted small">
                  exit {c.exit_code} · {c.duration_ms}ms
                </span>
              </div>
              {(c.stdout || c.stderr) && (
                <details className="collapse">
                  <summary className="muted small">output</summary>
                  {c.stdout && <pre>{c.stdout}</pre>}
                  {c.stderr && <pre className="stderr">{c.stderr}</pre>}
                </details>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
