import type { EnrichedWorkItem } from "../lib/aggregate";
import { relativeTime, shortSha } from "../lib/format";

// Highlights WorkItems blocked on a human decision. Approval actions stay in the
// existing ApprovalPanel (WorkItem detail); this panel only surfaces + links.
export default function HumanGatesPanel({ items }: { items: EnrichedWorkItem[] }) {
  if (items.length === 0) return null;
  return (
    <section className="panel gates">
      <h2>
        Human Gates pending <span className="count">{items.length}</span>
      </h2>
      <ul className="gate-list">
        {items.map((wi) => (
          <li key={wi.id} className="gate-item">
            <div className="gate-item-main">
              <a href={`#/workitems/${encodeURIComponent(wi.id)}`} className="gate-title">
                {wi.title}
              </a>
              <div className="muted small">
                {wi.project} · base {shortSha(wi.base_commit_sha)} · {relativeTime(wi.updated_at)}
              </div>
              {wi.description && <p className="gate-desc">{wi.description}</p>}
            </div>
            <a className="btn" href={`#/workitems/${encodeURIComponent(wi.id)}`}>
              Open →
            </a>
          </li>
        ))}
      </ul>
    </section>
  );
}
