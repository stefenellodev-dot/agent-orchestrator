import type { EnrichedWorkItem } from "../lib/aggregate";
import type { Phase } from "../types";
import WorkItemCard from "./WorkItemCard";

// Columns mirror the real state machine (internal/domain/workitem.go). This is
// a read-only board: no drag-and-drop, no client-side transitions.
const COLUMNS: { phase: Phase; label: string }[] = [
  { phase: "discovery", label: "Discovery" },
  { phase: "decision", label: "Decision" },
  { phase: "awaiting_approval", label: "Human Gate" },
  { phase: "implementation", label: "Implementing" },
  { phase: "validation", label: "Validating" },
  { phase: "complete", label: "Complete" },
];

export default function Kanban({ items }: { items: EnrichedWorkItem[] }) {
  const byPhase = (p: Phase) => items.filter((i) => i.current_phase === p);
  const attention = items.filter((i) => i.current_phase === "blocked" || i.current_phase === "failed");

  return (
    <div className="kanban-wrap">
      <div className="kanban" role="list">
        {COLUMNS.map((c) => {
          const list = byPhase(c.phase);
          return (
            <section key={c.phase} className={`kanban-col col-${c.phase}`} role="listitem">
              <header className="kanban-col-head">
                <span>{c.label}</span>
                <span className="count">{list.length}</span>
              </header>
              <div className="kanban-col-body">
                {list.map((wi) => (
                  <WorkItemCard key={wi.id} wi={wi} />
                ))}
                {list.length === 0 && <p className="empty muted small">—</p>}
              </div>
            </section>
          );
        })}
      </div>

      {attention.length > 0 && (
        <section className="kanban-attention">
          <header className="kanban-col-head">
            <span>Attention — blocked / failed</span>
            <span className="count">{attention.length}</span>
          </header>
          <div className="kanban-attention-body">
            {attention.map((wi) => (
              <WorkItemCard key={wi.id} wi={wi} />
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
