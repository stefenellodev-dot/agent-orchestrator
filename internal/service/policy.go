package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/stefenello/agent-orchestrator/internal/domain"
)

// policyReject records an auditable policy rejection and returns an error
// wrapping ErrPolicyViolation. Policy only rejects; it never auto-corrects or
// performs destructive remediation.
func (o *Orchestrator) policyReject(ctx context.Context, wi *domain.WorkItem, policy, operation, reason, cause string) error {
	_ = o.appendEvent(ctx, wi.ID, domain.EventPolicyRejected, domain.ActorSystem, map[string]any{
		"policy":    policy,
		"operation": operation,
		"reason":    reason,
		"cause":     cause,
	})
	if cause != "" {
		return fmt.Errorf("%w: %s: %s (%s)", ErrPolicyViolation, policy, reason, cause)
	}
	return fmt.Errorf("%w: %s: %s", ErrPolicyViolation, policy, reason)
}

// executionGuards run before Implementation. A rejection blocks the WorkItem for
// human triage.
func (o *Orchestrator) executionGuards(ctx context.Context, wi *domain.WorkItem) error {
	pc, registered := o.projects[wi.Project]
	if o.requireRegisteredProject && !registered {
		return o.policyReject(ctx, wi, "project", "execution", "project not registered", wi.Project)
	}
	if pc.RequireRuntime {
		if o.adapter == nil {
			return o.policyReject(ctx, wi, "runtime", "execution", "runtime adapter not configured", "")
		}
		pctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if _, err := o.adapter.Available(pctx); err != nil {
			return o.policyReject(ctx, wi, "runtime", "execution", "runtime unavailable", err.Error())
		}
	}
	if wi.WorktreePath == "" {
		return o.policyReject(ctx, wi, "worktree", "execution", "worktree not provisioned", "")
	}
	return nil
}

// completionGuards run before a WorkItem may reach Complete.
func (o *Orchestrator) completionGuards(ctx context.Context, wi *domain.WorkItem, out *domain.SessionOutput) error {
	pc := o.policyFor(wi.Project)

	if len(pc.ProtectedPaths) > 0 {
		for _, f := range o.changedFiles(ctx, wi) {
			if matchProtected(pc.ProtectedPaths, f) {
				return o.policyReject(ctx, wi, "protected_path", "completion", "protected path modified", f)
			}
		}
	}
	if pc.RequireValidation && countCommands(out) == 0 {
		return o.policyReject(ctx, wi, "require_validation", "completion", "no objective validation commands were executed", "")
	}
	return nil
}

func countCommands(o *domain.SessionOutput) int {
	if o == nil {
		return 0
	}
	return len(o.TestCommands) + len(o.LintCommands) + len(o.TypecheckCommands)
}

// changedFiles returns the files changed between the WorkItem's immutable base
// commit and HEAD in its worktree (objective, from git).
func (o *Orchestrator) changedFiles(ctx context.Context, wi *domain.WorkItem) []string {
	if wi.BaseCommitSHA == "" {
		return nil
	}
	out, err := runGitCommand(ctx, wi.WorktreePath, "diff", "--name-only", wi.BaseCommitSHA+"..HEAD")
	if err != nil {
		return nil
	}
	var files []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			files = append(files, l)
		}
	}
	return files
}

// matchProtected reports whether file matches any protected pattern (exact,
// simple glob, or directory prefix).
func matchProtected(patterns []string, file string) bool {
	for _, p := range patterns {
		if p == "" {
			continue
		}
		if file == p {
			return true
		}
		if ok, _ := filepath.Match(p, file); ok {
			return true
		}
		if strings.HasPrefix(file, strings.TrimSuffix(p, "/")+"/") {
			return true
		}
	}
	return false
}
