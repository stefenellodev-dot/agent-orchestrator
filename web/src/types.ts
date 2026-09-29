export type Phase =
  | "discovery"
  | "decision"
  | "awaiting_approval"
  | "implementation"
  | "validation"
  | "complete"
  | "blocked"
  | "failed";

export interface WorkItem {
  id: string;
  project: string;
  title: string;
  description: string;
  priority: string;
  status: Phase;
  current_phase: Phase;
  worktree_path: string;
  base_branch: string;
  base_commit_sha?: string;
  assignee?: string;
  metadata?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface ProjectValidation {
  test_commands?: string[];
  lint_commands?: string[];
  typecheck_commands?: string[];
}

export interface Project {
  name: string;
  repo_path: string;
  base_branch: string;
  validation?: ProjectValidation;
}

export interface Health {
  status: string;
  version: string;
  commit: string;
  build_time: string;
  opencode?: string;
  agents?: string;
}

export interface AuthorizationRecord {
  granted: boolean;
  granted_by: string;
  granted_at: string;
  approved_commit_sha?: string;
}

export interface Approval {
  user: string;
  decision: string;
  comment?: string;
  at: string;
  authorization?: AuthorizationRecord;
}

export interface Gate {
  id: string;
  work_item_id: string;
  phase: Phase;
  status: "pending" | "approved" | "rejected" | "changes_requested";
  approvals: Approval[];
  payload: {
    diff_preview?: string;
    risk_assessment?: string;
    evidence?: { type: string; summary: string }[];
  };
  created_at: string;
  resolved_at?: string;
}

export interface CommandResult {
  command: string;
  exit_code: number;
  stdout?: string;
  stderr?: string;
  duration_ms: number;
  passed: boolean;
}

export interface SessionOutput {
  agent_text?: string;
  base_commit_sha?: string;
  commit_sha?: string;
  diff?: string;
  diff_stat?: string;
  test_commands?: CommandResult[];
  lint_commands?: CommandResult[];
  typecheck_commands?: CommandResult[];
  validation_output?: string;
}

export interface Session {
  id: string;
  work_item_id: string;
  phase: Phase;
  opencode_session?: string;
  status: string;
  agent: string;
  prompt: string;
  exit_code: number;
  output?: SessionOutput;
  error?: string;
  started_at: string;
  completed_at?: string;
}

export interface Event {
  id: string;
  work_item_id: string;
  type: string;
  payload: Record<string, unknown>;
  actor: string;
  at: string;
}
