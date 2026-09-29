import type { Event } from "../types";
import { fmtTime } from "../lib/format";

function detail(e: Event): string {
  const p = e.payload ?? {};
  const parts: string[] = [];
  if (typeof p.phase === "string") parts.push(`phase ${p.phase}`);
  if (typeof p.error === "string") parts.push(p.error);
  if (typeof p.commit_sha === "string") parts.push(`commit ${String(p.commit_sha).slice(0, 10)}`);
  if (typeof p.exit_code === "number") parts.push(`exit ${p.exit_code}`);
  if (typeof p.by === "string") parts.push(`by ${p.by}`);
  if (typeof p.path === "string") parts.push(String(p.path).split("/").pop() ?? "");
  return parts.filter(Boolean).join(" · ");
}

// Chronological view built strictly from persisted events (no synthetic events).
export default function Timeline({ events }: { events: Event[] }) {
  const ordered = [...events].sort((a, b) => a.at.localeCompare(b.at));
  return (
    <ol className="timeline">
      {ordered.map((e) => (
        <li key={e.id} className="tl-item">
          <span className="tl-time mono small muted">{fmtTime(e.at)}</span>
          <span className="tl-dot" />
          <div className="tl-body">
            <span className={`feed-type type-${e.type.split(".")[0]}`}>{e.type}</span>
            <span className="muted small">{e.actor}</span>
            {detail(e) && <span className="tl-detail muted small">{detail(e)}</span>}
          </div>
        </li>
      ))}
      {ordered.length === 0 && <li className="muted small">No events.</li>}
    </ol>
  );
}
