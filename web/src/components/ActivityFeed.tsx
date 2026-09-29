import type { EventWithItem } from "../lib/aggregate";

export default function ActivityFeed({ events }: { events: EventWithItem[] }) {
  return (
    <section className="panel">
      <h2>
        Activity <span className="count">{events.length}</span>
      </h2>
      {events.length === 0 ? (
        <p className="muted small">No events yet.</p>
      ) : (
        <ul className="feed">
          {events.map((e) => (
            <li key={e.id} className="feed-item">
              <span className="feed-time muted mono small">{new Date(e.at).toLocaleTimeString()}</span>
              <span className={`feed-type type-${e.type.split(".")[0]}`}>{e.type}</span>
              <a href={`#/workitems/${encodeURIComponent(e.workItem.id)}`} className="feed-wi">
                {e.workItem.title}
              </a>
              <span className="muted small">{e.actor}</span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
