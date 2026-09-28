import { useState } from "react";
import { api } from "../api/client";
import type { Gate } from "../types";

export default function ApprovalPanel({
  workItemId,
  gate,
  onDone,
}: {
  workItemId: string;
  gate: Gate;
  onDone: () => void;
}) {
  const [user, setUser] = useState("");
  const [comment, setComment] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const resolved = gate.status === "approved" || gate.status === "rejected";

  async function act(fn: (id: string, user: string, comment: string) => Promise<unknown>) {
    if (!user.trim()) {
      setError("Your name is required to record the decision.");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await fn(workItemId, user.trim(), comment.trim());
      onDone();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="card approval">
      <h2>Human approval gate</h2>
      <p className="muted">
        Implementation is blocked until authorized. Gate status: <strong>{gate.status}</strong>
      </p>
      {gate.payload?.diff_preview && <pre className="diffpreview">{gate.payload.diff_preview}</pre>}

      {resolved ? (
        <p className="muted">This gate has been resolved.</p>
      ) : (
        <>
          <label>
            Approver
            <input value={user} onChange={(e) => setUser(e.target.value)} placeholder="your name" />
          </label>
          <label>
            Comment
            <textarea value={comment} onChange={(e) => setComment(e.target.value)} rows={3} />
          </label>
          {error && <p className="error">{error}</p>}
          <div className="actions">
            <button disabled={busy} onClick={() => act(api.approve)}>
              Approve &amp; authorize
            </button>
            <button disabled={busy} className="secondary" onClick={() => act(api.requestChanges)}>
              Request changes
            </button>
            <button disabled={busy} className="danger" onClick={() => act(api.reject)}>
              Reject
            </button>
          </div>
        </>
      )}
    </section>
  );
}
