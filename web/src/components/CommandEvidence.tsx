import type { Session } from "../types";
import { fmtTime, phaseLabel } from "../lib/format";
import { allCommands } from "../lib/evidence";

export default function CommandEvidence({ sessions }: { sessions: Session[] }) {
  const rows = allCommands(sessions);
  if (rows.length === 0) return <p className="muted small">No command results recorded.</p>;

  return (
    <ul className="cmd-results">
      {rows.map(({ session, command }, i) => (
        <li key={`${session.id}:${i}`} className={`cmd-result ${command.passed ? "pass" : "fail"}`}>
          <div className="cmd-line">
            <span className={`chip phase phase-${session.phase}`}>{phaseLabel(session.phase)}</span>
            <span className={`chip ${command.exit_code === 0 ? "ok" : "fail"}`}>
              {command.exit_code === 0 ? "pass" : "fail"}
            </span>
            <code>{command.command}</code>
            <span className="muted small">
              exit {command.exit_code} · {command.duration_ms}ms
            </span>
          </div>
          <div className="muted small">
            session <span className="mono">{session.id.slice(0, 10)}</span> · {session.agent} ·{" "}
            <span className="tag derived">SESSION TIMESTAMP — DERIVED</span> {fmtTime(session.started_at)}
          </div>
          {(command.stdout || command.stderr) && (
            <details className="collapse">
              <summary className="muted small">
                stdout/stderr (
                {Math.round(((command.stdout?.length ?? 0) + (command.stderr?.length ?? 0)) / 1024)} KB)
              </summary>
              {command.stdout && <pre>{command.stdout}</pre>}
              {command.stderr && <pre className="stderr">{command.stderr}</pre>}
            </details>
          )}
        </li>
      ))}
    </ul>
  );
}
