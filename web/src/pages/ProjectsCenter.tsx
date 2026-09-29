import { useEffect, useState } from "react";
import { loadProjectsData, type ProjectsData } from "../lib/aggregate";
import type { Phase, WorkItem } from "../types";
import { isActivePhase, phaseLabel, relativeTime } from "../lib/format";

const PHASES: Phase[] = [
  "discovery",
  "decision",
  "awaiting_approval",
  "implementation",
  "validation",
  "complete",
];

function Stats({ items }: { items: WorkItem[] }) {
  const active = items.filter((w) => isActivePhase(w.current_phase)).length;
  const last = items.map((w) => w.updated_at).sort().slice(-1)[0];
  return (
    <div className="muted small">
      {items.length} WorkItems · {active} active · last activity {last ? relativeTime(last) : "—"}
      <div className="row wrap gap phase-dist">
        {PHASES.map((p) => {
          const n = items.filter((w) => w.current_phase === p).length;
          return n > 0 ? (
            <span key={p} className={`chip phase phase-${p}`}>
              {phaseLabel(p)} {n}
            </span>
          ) : null;
        })}
      </div>
    </div>
  );
}

function ItemList({ items }: { items: WorkItem[] }) {
  return (
    <ul className="mini-list">
      {items.map((w) => (
        <li key={w.id}>
          <a href={`#/workitems/${encodeURIComponent(w.id)}`}>{w.title}</a>{" "}
          <span className={`chip phase phase-${w.current_phase}`}>{phaseLabel(w.current_phase)}</span>{" "}
          <a href={`#/evidence/${encodeURIComponent(w.id)}`} className="muted small">
            evidence
          </a>
        </li>
      ))}
    </ul>
  );
}

export default function ProjectsCenter() {
  const [data, setData] = useState<ProjectsData | null>(null);

  useEffect(() => {
    loadProjectsData().then(setData);
  }, []);

  if (!data) return <p className="muted">Loading…</p>;

  const byProject = new Map<string, WorkItem[]>();
  for (const w of data.workitems) {
    const arr = byProject.get(w.project) ?? [];
    arr.push(w);
    byProject.set(w.project, arr);
  }
  const registered = new Set(data.projects.map((p) => p.name));
  const observed = [...byProject.keys()].filter((n) => !registered.has(n)).sort();

  const validationOf = (cmds?: string[]) => (cmds && cmds.length ? cmds.join(" · ") : "—");

  return (
    <div className="command-center">
      <h1>Projects</h1>

      {data.projects.length === 0 && (
        <p className="muted small">No projects registered in /api/projects.</p>
      )}

      {data.projects.map((p) => {
        const items = byProject.get(p.name) ?? [];
        const v = p.validation ?? {};
        return (
          <section key={p.name} className="panel">
            <h2>
              {p.name} <span className="tag persisted">REGISTERED</span>
            </h2>
            <dl className="cc-identity">
              <div>
                <dt>repo_path</dt>
                <dd className="mono small">{p.repo_path || "—"}</dd>
              </div>
              <div>
                <dt>base branch</dt>
                <dd className="mono">{p.base_branch || "—"}</dd>
              </div>
              <div>
                <dt>validation commands</dt>
                <dd className="mono small">
                  {validationOf([
                    ...(v.test_commands ?? []),
                    ...(v.lint_commands ?? []),
                    ...(v.typecheck_commands ?? []),
                  ])}
                </dd>
              </div>
            </dl>
            <Stats items={items} />
            {items.length > 0 ? (
              <ItemList items={items} />
            ) : (
              <p className="muted small">No WorkItems for this project.</p>
            )}
          </section>
        );
      })}

      {observed.length > 0 && (
        <section className="panel">
          <h2>
            Observed projects <span className="tag derived">UNREGISTERED</span>
          </h2>
          <p className="muted small">
            Referenced by WorkItems but not present in /api/projects. repo_path, base branch and
            validation commands are unknown and not inferred.
          </p>
          {observed.map((name) => {
            const items = byProject.get(name) ?? [];
            return (
              <div key={name} className="observed">
                <strong>{name}</strong>{" "}
                <span className="muted small">(repo_path / branch / validation: unknown)</span>
                <Stats items={items} />
                <ItemList items={items} />
              </div>
            );
          })}
        </section>
      )}

      {data.errors.length > 0 && <p className="muted small">Partial data: {data.errors.join("; ")}</p>}
    </div>
  );
}
