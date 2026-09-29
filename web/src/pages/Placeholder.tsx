export default function Placeholder({ title, note }: { title: string; note: string }) {
  return (
    <div className="placeholder">
      <h1>{title}</h1>
      <p className="muted">{note}</p>
      <p className="muted small">Planned for a later WorkItem (UI-03/04/05).</p>
      <p>
        <a href="#/">← Back to Mission Control</a>
      </p>
    </div>
  );
}
