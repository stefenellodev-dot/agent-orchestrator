import { phaseLabel } from "../lib/format";

export default function PhaseBadge({ phase }: { phase: string }) {
  return <span className={`chip phase phase-${phase}`}>{phaseLabel(phase)}</span>;
}
