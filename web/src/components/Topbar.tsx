import type { Health } from "../types";
import { shortSha } from "../lib/format";
import type { Theme } from "../hooks/useTheme";

const LINKS = [
  { href: "#/", label: "Mission Control" },
  { href: "#/workitems", label: "Work Items" },
  { href: "#/projects", label: "Projects" },
  { href: "#/agents", label: "Agents" },
  { href: "#/evidence", label: "Evidence" },
  { href: "#/operations", label: "Operations" },
];

// Highlights the section for both the exact route and its subroutes
// (e.g. #/workitems/:id keeps "Work Items" active).
function isActive(current: string, href: string): boolean {
  if (href === "#/") return current === "#/" || current === "";
  return current === href || current.startsWith(href + "/");
}

export default function Topbar({
  health,
  theme,
  onToggleTheme,
  current,
}: {
  health: Health | null;
  theme: Theme;
  onToggleTheme: () => void;
  current: string;
}) {
  return (
    <header className="topbar">
      <a href="#/" className="brand">
        Agent Orchestrator
      </a>
      <nav className="nav" aria-label="Primary">
        {LINKS.map((l) => (
          <a
            key={l.href}
            href={l.href}
            className={isActive(current, l.href) ? "active" : ""}
            aria-current={isActive(current, l.href) ? "page" : undefined}
          >
            {l.label}
          </a>
        ))}
      </nav>
      <div className="topbar-right">
        <span className={`pill ${health ? "ok" : "bad"}`} title="Orchestrator /healthz">
          {health ? "online" : "offline"}
        </span>
        {health && (
          <span className="mono small muted" title="version · commit · opencode">
            {health.version} · {shortSha(health.commit)} · oc {health.opencode ?? "—"}
          </span>
        )}
        <button className="ghost" onClick={onToggleTheme} aria-label="Toggle color theme">
          {theme === "dark" ? "Light" : "Dark"}
        </button>
      </div>
    </header>
  );
}
