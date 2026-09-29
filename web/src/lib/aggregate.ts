import { api } from "../api/client";
import type { Event, Health, Project, Session, WorkItem } from "../types";

export interface EnrichedWorkItem extends WorkItem {
  sessions: Session[];
  latest?: Session;
  lastValidation?: Session;
}

export type SessionWithItem = Session & { workItem: WorkItem };
export type EventWithItem = Event & { workItem: WorkItem };

export interface MissionData {
  health: Health | null;
  projects: Project[];
  workitems: EnrichedWorkItem[];
  sessions: SessionWithItem[];
  events: EventWithItem[];
  pendingGates: EnrichedWorkItem[];
  errors: string[];
}

function newestFirst<T extends { started_at?: string; at?: string }>(a: T, b: T): number {
  const ka = a.started_at ?? a.at ?? "";
  const kb = b.started_at ?? b.at ?? "";
  return kb.localeCompare(ka);
}

// Aggregates the existing per-WorkItem endpoints into a single Mission Control
// view. UI-02 accepts frontend N+1 aggregation (no backend changes).
export async function loadMissionData(): Promise<MissionData> {
  const errors: string[] = [];

  const health = await api.health().catch((e) => {
    errors.push(`health: ${e}`);
    return null;
  });
  const projects = await api.listProjects().catch((e) => {
    errors.push(`projects: ${e}`);
    return [] as Project[];
  });
  const items = await api.listWorkItems().catch((e) => {
    errors.push(`workitems: ${e}`);
    return [] as WorkItem[];
  });

  const enriched = await Promise.all(
    items.map(async (wi) => {
      const [sessions, events] = await Promise.all([
        api.listSessions(wi.id).catch(() => [] as Session[]),
        api.listEvents(wi.id).catch(() => [] as Event[]),
      ]);
      const latest = [...sessions].sort(newestFirst)[0];
      const lastValidation = sessions
        .filter((s) => s.phase === "validation")
        .sort(newestFirst)[0];
      return { wi, sessions, events, latest, lastValidation };
    }),
  );

  const workitems: EnrichedWorkItem[] = enriched.map((e) => ({
    ...e.wi,
    sessions: e.sessions,
    latest: e.latest,
    lastValidation: e.lastValidation,
  }));

  const sessions: SessionWithItem[] = enriched.flatMap((e) =>
    e.sessions.map((s) => ({ ...s, workItem: e.wi })),
  );

  const events: EventWithItem[] = enriched
    .flatMap((e) => e.events.map((ev) => ({ ...ev, workItem: e.wi })))
    .sort((a, b) => b.at.localeCompare(a.at))
    .slice(0, 60);

  const pendingGates = workitems.filter((w) => w.current_phase === "awaiting_approval");

  return { health, projects, workitems, sessions, events, pendingGates, errors };
}

export interface ProjectsData {
  projects: Project[];
  workitems: WorkItem[];
  errors: string[];
}

// Projects Center: registered projects (/api/projects) + WorkItems to associate
// and to surface projects observed but not registered.
export async function loadProjectsData(): Promise<ProjectsData> {
  const errors: string[] = [];
  const projects = await api.listProjects().catch((e) => {
    errors.push(`projects: ${e}`);
    return [] as Project[];
  });
  const workitems = await api.listWorkItems().catch((e) => {
    errors.push(`workitems: ${e}`);
    return [] as WorkItem[];
  });
  return { projects, workitems, errors };
}

export interface AgentsData {
  health: Health | null;
  workitems: WorkItem[];
  sessions: SessionWithItem[];
  errors: string[];
}

// Agents Center: available agents come from /healthz; execution evidence is the
// aggregated sessions across WorkItems (N+1 accepted).
export async function loadAgentsData(): Promise<AgentsData> {
  const errors: string[] = [];
  const health = await api.health().catch((e) => {
    errors.push(`health: ${e}`);
    return null;
  });
  const workitems = await api.listWorkItems().catch((e) => {
    errors.push(`workitems: ${e}`);
    return [] as WorkItem[];
  });
  const sessions: SessionWithItem[] = (
    await Promise.all(
      workitems.map(async (wi) => {
        const s = await api.listSessions(wi.id).catch(() => [] as Session[]);
        return s.map((x) => ({ ...x, workItem: wi }));
      }),
    )
  ).flat();
  return { health, workitems, sessions, errors };
}
