import type { Event, Gate, Health, Project, Session, WorkItem } from "../types";

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
  });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`${res.status}: ${body}`);
  }
  return (await res.json()) as T;
}

export const api = {
  health: () => req<Health>(`/healthz`),

  listWorkItems: (project?: string) =>
    req<WorkItem[]>(`/api/workitems${project ? `?project=${encodeURIComponent(project)}` : ""}`),

  getWorkItem: (id: string) => req<WorkItem>(`/api/workitems/${id}`),

  listEvents: (id: string) => req<Event[]>(`/api/workitems/${id}/events`),

  listSessions: (id: string) => req<Session[]>(`/api/workitems/${id}/sessions`),

  getGate: (id: string) => req<Gate>(`/api/workitems/${id}/gate`),

  listProjects: () => req<Project[]>(`/api/projects`),

  approve: (id: string, user: string, comment: string) =>
    req<unknown>(`/api/workitems/${id}/approve`, {
      method: "POST",
      body: JSON.stringify({ user, comment }),
    }),

  requestChanges: (id: string, user: string, comment: string) =>
    req<unknown>(`/api/workitems/${id}/request-changes`, {
      method: "POST",
      body: JSON.stringify({ user, comment }),
    }),

  reject: (id: string, user: string, comment: string) =>
    req<unknown>(`/api/workitems/${id}/reject`, {
      method: "POST",
      body: JSON.stringify({ user, comment }),
    }),
};
