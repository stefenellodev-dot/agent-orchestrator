import type { CommandResult, Session } from "../types";

export function byPhase(sessions: Session[], phase: string): Session[] {
  return sessions
    .filter((s) => s.phase === phase)
    .sort((a, b) => b.started_at.localeCompare(a.started_at));
}

export function implementationSession(sessions: Session[]): Session | undefined {
  return byPhase(sessions, "implementation")[0];
}

export function validationSession(sessions: Session[]): Session | undefined {
  return byPhase(sessions, "validation")[0];
}

export function decisionSession(sessions: Session[]): Session | undefined {
  return byPhase(sessions, "decision")[0];
}

// The implementation commit is NOT a first-class WorkItem field; it is derived
// from the most recent implementation session. Always label it as DERIVED.
export function implementationCommit(sessions: Session[]): string | undefined {
  const impl = implementationSession(sessions);
  if (impl?.output?.commit_sha) return impl.output.commit_sha;
  return validationSession(sessions)?.output?.commit_sha;
}

export function commandsOf(s: Session): CommandResult[] {
  return [
    ...(s.output?.test_commands ?? []),
    ...(s.output?.lint_commands ?? []),
    ...(s.output?.typecheck_commands ?? []),
  ];
}

export function allCommands(sessions: Session[]): { session: Session; command: CommandResult }[] {
  return sessions.flatMap((s) => commandsOf(s).map((command) => ({ session: s, command })));
}

export interface ValidationOutcome {
  state: "PASS" | "FAIL" | "NOT RUN";
  checks: number;
  passed: number;
  failed: number;
  totalMs: number;
  session?: Session;
}

// PASS/FAIL is decided strictly by exit codes (never by textual output).
export function validationOutcome(sessions: Session[]): ValidationOutcome {
  const v = validationSession(sessions);
  if (!v) return { state: "NOT RUN", checks: 0, passed: 0, failed: 0, totalMs: 0 };
  const cmds = commandsOf(v);
  const passed = cmds.filter((c) => c.exit_code === 0).length;
  const failed = cmds.filter((c) => c.exit_code !== 0).length;
  const totalMs = cmds.reduce((a, c) => a + (c.duration_ms || 0), 0);
  const ok = cmds.length > 0 ? failed === 0 : v.exit_code === 0;
  return { state: ok ? "PASS" : "FAIL", checks: cmds.length, passed, failed, totalMs, session: v };
}
