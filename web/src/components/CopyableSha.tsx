import { useState } from "react";

// Compact SHA presentation that copies the full value on click.
export default function CopyableSha({ sha, label }: { sha?: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  if (!sha) return <span className="muted">—</span>;

  const copy = () => {
    navigator.clipboard
      ?.writeText(sha)
      .then(() => {
        setCopied(true);
        setTimeout(() => setCopied(false), 1200);
      })
      .catch(() => {});
  };

  return (
    <button type="button" className="sha" onClick={copy} title={sha}>
      <span className="mono">{sha.slice(0, 10)}</span>
      {label && <span className="muted small">{label}</span>}
      <span className="sha-copy muted small">{copied ? "copied" : "copy"}</span>
    </button>
  );
}
