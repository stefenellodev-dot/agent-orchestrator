import type { Phase } from "../types";
import { phaseLabel } from "../lib/format";

// Mirrors the real state machine. Read-only visualisation: the backend owns all
// transitions; blocked/failed are shown as attention, never as a step.
const FLOW: Phase[] = [
  "discovery",
  "decision",
  "awaiting_approval",
  "implementation",
  "validation",
  "complete",
];

export default function StateFlow({ current }: { current: Phase }) {
  const attention = current === "blocked" || current === "failed";
  const idx = FLOW.indexOf(current);

  return (
    <div className="stateflow">
      <ol className="sf-track">
        {FLOW.map((p, i) => {
          let state: "done" | "current" | "pending" = "pending";
          if (!attention && idx >= 0) {
            state = i < idx ? "done" : i === idx ? "current" : "pending";
          }
          return (
            <li key={p} className={`sf-step ${state} ${p === "awaiting_approval" ? "gate" : ""}`}>
              <span className="sf-dot" />
              <span className="sf-label">{phaseLabel(p)}</span>
            </li>
          );
        })}
      </ol>
      {attention && <div className={`sf-attention ${current}`}>Attention · {phaseLabel(current)}</div>}
    </div>
  );
}
