package domain

import (
	"context"
	"errors"
	"time"
)

// Runtime error classification (R4).
var (
	// ErrRuntimeUnavailable means the runtime binary/probe could not be used.
	ErrRuntimeUnavailable = errors.New("runtime unavailable")
	// ErrProcessStart means the runtime process could not be started.
	ErrProcessStart = errors.New("runtime process start failed")
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
	// TimedOut/Canceled distinguish interruption from a task/execution failure.
	// A non-zero ExitCode without these flags is an execution (task) failure.
	TimedOut bool
	Canceled bool
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

// Runtime is the execution contract. OpenCode is the current implementation;
// the interface is deliberately runtime-agnostic so other runtimes can be added
// later without changing the orchestrator.
type Runtime interface {
	// Name identifies the runtime (e.g. "opencode").
	Name() string
	// Available performs a safe availability/capability check. A non-nil error
	// (wrapping ErrRuntimeUnavailable) means the runtime cannot be used.
	Available(ctx context.Context) (CLICapabilities, error)
	// Run executes one phase. A non-zero process exit is evidence in
	// RunResult.ExitCode (execution/task failure), not an error. An error means
	// the process could not be started at all (wrapping ErrProcessStart).
	Run(ctx context.Context, req RunRequest) (*RunResult, error)
}

// OpenCodeAdapter is retained as an alias for the runtime contract.
type OpenCodeAdapter = Runtime
