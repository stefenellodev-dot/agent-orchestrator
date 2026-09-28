package domain

import (
	"context"
	"time"
)

// RunRequest describes a single OpenCode phase execution.
type RunRequest struct {
	// WorktreePath is the WorkItem's single worktree, reused across phases.
	WorktreePath string
	// BaseBranch/BaseCommitSHA anchor the objective diff evidence.
	BaseBranch    string
	BaseCommitSHA string
	Phase         Phase
	Prompt        string
	Agent         string
	// Model is the explicit provider/model. May be empty.
	Model string
	// SessionID and Fork are best-effort. MVP correctness must not depend on
	// session continuity; callers may leave them empty.
	SessionID string
	Fork      bool
	EnvVars   map[string]string
	Timeout   time.Duration
}

// RunResult is the outcome of one OpenCode execution.
type RunResult struct {
	ExitCode        int
	Stdout          string
	Stderr          string
	OpenCodeSession string
	// AgentText is the assistant's final text, extracted from the output when
	// the format is understood. Best-effort.
	AgentText string
	// Objective evidence, collected by the adapter (not self-reported).
	BaseCommitSHA string
	CommitSHA     string
	Diff          string
	DiffStat      string
	// Parsed results when the output format is understood; best-effort.
	TestResults      []TestResult
	LintResults      []LintResult
	TypecheckResults []TypecheckResult
	Artifacts        []Artifact
}

// CLICapabilities is the verified surface of the installed OpenCode CLI.
// It is discovered at startup; nothing here is assumed.
type CLICapabilities struct {
	Version          string
	SupportsJSON     bool
	SupportsSession  bool
	SupportsContinue bool
	SupportsFork     bool
	SupportsAgent    bool
	SupportsDir      bool
	SupportsAttach   bool
	SupportsModel    bool
	SupportedAgents  []string
}

// OpenCodeAdapter abstracts how a phase is executed, so local (direct
// `opencode run`) and Piave (systemd/Podman) differ only in the launcher.
type OpenCodeAdapter interface {
	Run(ctx context.Context, req RunRequest) (*RunResult, error)
	ValidateCLI(ctx context.Context) (CLICapabilities, error)
}
