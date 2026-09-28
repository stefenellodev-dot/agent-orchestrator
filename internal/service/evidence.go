package service

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/stefenello/agent-orchestrator/internal/domain"
)

// ProjectValidation is the orchestrator-owned set of objective commands run to
// validate a WorkItem. Consumer repositories need no changes.
type ProjectValidation struct {
	TestCommands      []string
	LintCommands      []string
	TypecheckCommands []string
}

// EvidenceCollector produces objective, reproducible evidence: git identity of
// the change plus the exit codes of the configured validation commands.
type EvidenceCollector struct {
	timeout time.Duration
}

func NewEvidenceCollector() *EvidenceCollector {
	return &EvidenceCollector{timeout: 5 * time.Minute}
}

// Collect gathers git evidence and runs the validation commands in the worktree.
func (c *EvidenceCollector) Collect(ctx context.Context, worktree, baseBranch string, pv ProjectValidation) (*domain.SessionOutput, error) {
	out := &domain.SessionOutput{}

	out.BaseCommitSHA, _ = revParse(ctx, worktree)
	if baseBranch != "" {
		if baseSHA, err := runGitCommand(ctx, worktree, "rev-parse", baseBranch); err == nil {
			out.BaseCommitSHA = strings.TrimSpace(baseSHA)
		}
	}
	out.CommitSHA, _ = revParse(ctx, worktree)

	if out.BaseCommitSHA != "" && out.CommitSHA != "" && out.BaseCommitSHA != out.CommitSHA {
		if diff, err := runGitCommand(ctx, worktree, "diff", out.BaseCommitSHA+".."+out.CommitSHA); err == nil {
			out.Diff = diff
		}
		if stat, err := runGitCommand(ctx, worktree, "diff", "--stat", out.BaseCommitSHA+".."+out.CommitSHA); err == nil {
			out.DiffStat = stat
		}
	}

	for _, cmd := range pv.TestCommands {
		out.TestCommands = append(out.TestCommands, c.runCommand(ctx, worktree, cmd))
	}
	for _, cmd := range pv.LintCommands {
		out.LintCommands = append(out.LintCommands, c.runCommand(ctx, worktree, cmd))
	}
	for _, cmd := range pv.TypecheckCommands {
		out.TypecheckCommands = append(out.TypecheckCommands, c.runCommand(ctx, worktree, cmd))
	}
	return out, nil
}

// runCommand executes a single shell command in the worktree and records its
// objective outcome.
func (c *EvidenceCollector) runCommand(ctx context.Context, dir, command string) domain.CommandResult {
	timeout := c.timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(runCtx, "sh", "-c", command)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	exitCode := 0
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return domain.CommandResult{
		Command:    command,
		ExitCode:   exitCode,
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		DurationMS: time.Since(start).Milliseconds(),
		Passed:     exitCode == 0,
	}
}
