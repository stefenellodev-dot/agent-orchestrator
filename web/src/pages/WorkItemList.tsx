import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { WorkItem } from "../types";

export function PhaseBadge({ phase }: { phase: string }) {
  return <span className={`badge badge-${phase}`}>{phase.replace(/_/g, " ")}</span>;
}

export default function WorkItemList() {
  const [items, setItems] = useState<WorkItem[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api
      .listWorkItems()
      .then(setItems)
      .catch((e) => setError(String(e)))
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <p className="muted">Loading…</p>;
  if (error) return <p className="error">{error}</p>;
  if (items.length === 0) return <p className="muted">No WorkItems yet.</p>;

  return (
    <table className="table">
      <thead>
        <tr>
          <th>Title</th>
          <th>Project</th>
          <th>Phase</th>
          <th>Updated</th>
        </tr>
      </thead>
      <tbody>
        {items.map((wi) => (
          <tr key={wi.id}>
            <td>
              <a href={`#/workitems/${encodeURIComponent(wi.id)}`}>{wi.title}</a>
            </td>
            <td>{wi.project}</td>
            <td>
              <PhaseBadge phase={wi.current_phase} />
            </td>
            <td className="muted">{new Date(wi.updated_at).toLocaleString()}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
