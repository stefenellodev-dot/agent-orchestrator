import type { SessionWithItem } from "../lib/aggregate";
import { relativeTime } from "../lib/format";

export default function ValidationsPanel({ sessions }: { sessions: SessionWithItem[] }) {
  const vals = sessions
    .filter((s) => s.phase === "validation")
    .sort((a, b) => (b.completed_at ?? b.started_at).localeCompare(a.completed_at ?? a.started_at))
    .slice(0, 10);

  return (
    <section className="panel">
      <h2>
        Recent validations <span className="count">{vals.length}</span>
      </h2>
      {vals.length === 0 ? (
        <p className="muted small">No validations yet.</p>
      ) : (
        <ul className="val-list">
          {vals.map((s) => {
            const cmds = [
              ...(s.output?.test_commands ?? []),
              ...(s.output?.lint_commands ?? []),
              ...(s.output?.typecheck_commands ?? []),
            ];
            return (
              <li key={s.id} className="val-item">
                <div className="val-head">
                  <a href={`#/workitems/${encodeURIComponent(s.workItem.id)}`}>{s.workItem.title}</a>
                  <span className={`chip ${s.exit_code === 0 ? "ok" : "fail"}`}>
                    {s.exit_code === 0 ? "PASS" : "FAIL"} · exit {s.exit_code}
                  </span>
                  <span className="muted small">{relativeTime(s.completed_at ?? s.started_at)}</span>
                </div>
                {cmds.length > 0 && (
                  <ul className="cmd-list">
                    {cmds.map((c, i) => (
                      <li key={i} className={`cmd ${c.passed ? "pass" : "fail"}`}>
                        <code>{c.command}</code> → exit {c.exit_code}
                      </li>
                    ))}
                  </ul>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
