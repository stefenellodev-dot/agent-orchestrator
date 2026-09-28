package domain

import "time"

// TestResult is one objective test outcome.
type TestResult struct {
	Name     string  `json:"name"`
	Status   string  `json:"status"` // "pass" | "fail" | "skip" | "error"
	Duration float64 `json:"duration_seconds,omitempty"`
	Output   string  `json:"output,omitempty"`
}

type LintResult struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Rule    string `json:"rule,omitempty"`
}

type TypecheckResult struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

// CommandResult is the raw, reproducible outcome of a single command.
type CommandResult struct {
	Command    string   `json:"command"`
	ExitCode   int      `json:"exit_code"`
	Stdout     string   `json:"stdout,omitempty"`
	Stderr     string   `json:"stderr,omitempty"`
	DurationMS int64    `json:"duration_ms"`
	Passed     bool     `json:"passed"`
}

type Artifact struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum,omitempty"`
}

// SessionOutput holds objective, reproducible evidence produced by a phase.
// Agent self-reported confidence is NOT part of the completion decision.
type SessionOutput struct {
	// Git evidence
	BaseCommitSHA string `json:"base_commit_sha,omitempty"`
	CommitSHA     string `json:"commit_sha,omitempty"`
	Diff          string `json:"diff,omitempty"`
	DiffStat      string `json:"diff_stat,omitempty"`

	// Command evidence
	TestCommands      []CommandResult   `json:"test_commands,omitempty"`
	LintCommands      []CommandResult   `json:"lint_commands,omitempty"`
	TypecheckCommands []CommandResult   `json:"typecheck_commands,omitempty"`
	ValidationOutput  string            `json:"validation_output,omitempty"`
	TestResults       []TestResult      `json:"test_results,omitempty"`
	LintResults       []LintResult      `json:"lint_results,omitempty"`
	TypecheckResults  []TypecheckResult `json:"typecheck_results,omitempty"`

	Artifacts []Artifact `json:"artifacts,omitempty"`
}

// AllValidationPassed reports whether every recorded validation command exited 0.
// An empty validation set is NOT considered passed (must be explicit).
func (o *SessionOutput) AllValidationPassed() bool {
	cmds := append(append([]CommandResult{}, o.TestCommands...), o.LintCommands...)
	cmds = append(cmds, o.TypecheckCommands...)
	if len(cmds) == 0 {
		return false
	}
	for _, c := range cmds {
		if c.ExitCode != 0 {
			return false
		}
	}
	return true
}

// Evidence is a durable, human-readable reference to produced evidence.
type Evidence struct {
	SessionID SessionID  `json:"session_id"`
	Type      string     `json:"type"`
	Content   string     `json:"content"`
	Artifacts []Artifact `json:"artifacts,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}
