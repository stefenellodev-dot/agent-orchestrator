import type { MissionData } from "../lib/aggregate";
import { isActivePhase } from "../lib/format";

function Stat({ label, value, tone }: { label: string; value: number; tone?: string }) {
  return (
    <div className={`stat ${tone ?? ""}`}>
      <div className="stat-value">{value}</div>
      <div className="stat-label">{label}</div>
    </div>
  );
}

export default function SummaryBar({ data }: { data: MissionData }) {
  const total = data.workitems.length;
  const active = data.workitems.filter((w) => isActivePhase(w.current_phase)).length;
  const gates = data.pendingGates.length;
  const validations = data.sessions.filter((s) => s.phase === "validation").length;
  return (
    <div className="summary">
      <Stat label="WorkItems" value={total} />
      <Stat label="Active" value={active} />
      <Stat label="Human Gates" value={gates} tone={gates > 0 ? "warn" : undefined} />
      <Stat label="Validations" value={validations} />
      <Stat label="Projects" value={data.projects.length} />
    </div>
  );
}
