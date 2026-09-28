package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/stefenello/agent-orchestrator/internal/domain"
)

// Drive runs the WorkItem workflow until it reaches a terminal phase or stops
// at the human gate. It is safe to call once per WorkItem.
func (o *Orchestrator) Drive(ctx context.Context, id domain.WorkItemID) error {
	for {
		wi, err := o.store.GetWorkItem(ctx, id)
		if err != nil {
			return err
		}
		phase := wi.CurrentPhase

		if phase.IsTerminal() {
			return nil
		}

		if phase == domain.PhaseAwaitingApproval {
			if !o.autoApprove {
				return nil // pause for a human decision
			}
			if err := o.grantAuthorization(ctx, wi, "auto-approve(tests)"); err != nil {
				return err
			}
			continue
		}

		if !phase.RequiresOpenCodeRun() {
			return fmt.Errorf("cannot drive phase %q", phase)
		}

		res, err := o.executePhase(ctx, wi)
		if err != nil {
			return o.markFailed(ctx, wi, err)
		}

		if phase == domain.PhaseValidation && res.ExitCode != 0 {
			return o.markFailed(ctx, wi, fmt.Errorf("validation failed with exit code %d", res.ExitCode))
		}

		next := phase.Next()
		if next == domain.PhaseAwaitingApproval {
			if err := o.createGate(ctx, wi, res); err != nil {
				return err
			}
		}
		if err := o.updatePhase(ctx, wi, next); err != nil {
			return err
		}
		if next == domain.PhaseComplete {
			o.cleanupWorktree(ctx, wi)
			return nil
		}
	}
}

// executePhase records a Session, invokes OpenCode for the current phase, and
// stores the objective evidence returned by the adapter.
func (o *Orchestrator) executePhase(ctx context.Context, wi *domain.WorkItem) (*domain.RunResult, error) {
	if o.adapter == nil {
		return nil, errors.New("no OpenCode adapter configured")
	}

	agent := o.agentFor(wi.CurrentPhase)
	prompt := PromptFor(wi, wi.CurrentPhase)

	sess := &domain.Session{
		ID:         domain.NewSessionID(),
		WorkItemID: wi.ID,
		Phase:      wi.CurrentPhase,
		Status:     domain.SessionRunning,
		Agent:      agent,
		Prompt:     prompt,
		StartedAt:  o.now(),
	}
	if err := o.store.CreateSession(ctx, sess); err != nil {
		return nil, err
	}
	_ = o.appendEvent(ctx, wi.ID, domain.EventSessionStarted, domain.ActorSystem, map[string]any{
		"session_id": string(sess.ID),
		"phase":      string(wi.CurrentPhase),
		"agent":      agent,
	})

	res, runErr := o.adapter.Run(ctx, domain.RunRequest{
		WorktreePath: wi.WorktreePath,
		BaseBranch:   wi.BaseBranch,
		Phase:        wi.CurrentPhase,
		Prompt:       prompt,
		Agent:        agent,
		Timeout:      o.phaseTimeout,
	})

	completed := o.now()
	sess.CompletedAt = &completed

	if runErr != nil {
		sess.Status = domain.SessionFailed
		sess.Error = runErr.Error()
		_ = o.store.UpdateSession(ctx, sess)
		_ = o.appendEvent(ctx, wi.ID, domain.EventSessionFailed, domain.ActorSystem, map[string]any{
			"session_id": string(sess.ID),
			"error":      runErr.Error(),
		})
		return nil, runErr
	}

	sess.ExitCode = res.ExitCode
	sess.OpenCodeSession = res.OpenCodeSession
	sess.Output = &domain.SessionOutput{
		BaseCommitSHA:    res.BaseCommitSHA,
		CommitSHA:        res.CommitSHA,
		Diff:             res.Diff,
		DiffStat:         res.DiffStat,
		ValidationOutput: res.Stdout,
		TestResults:      res.TestResults,
		LintResults:      res.LintResults,
		TypecheckResults: res.TypecheckResults,
		Artifacts:        res.Artifacts,
	}
	if res.ExitCode == 0 {
		sess.Status = domain.SessionCompleted
	} else {
		sess.Status = domain.SessionFailed
	}
	if err := o.store.UpdateSession(ctx, sess); err != nil {
		return nil, err
	}
	_ = o.appendEvent(ctx, wi.ID, domain.EventSessionCompleted, domain.ActorSystem, map[string]any{
		"session_id": string(sess.ID),
		"exit_code":  res.ExitCode,
	})
	return res, nil
}

// createGate opens the human approval gate after Decision completes.
func (o *Orchestrator) createGate(ctx context.Context, wi *domain.WorkItem, res *domain.RunResult) error {
	gate := &domain.Gate{
		ID:         domain.NewGateID(),
		WorkItemID: wi.ID,
		Phase:      domain.PhaseAwaitingApproval,
		Status:     domain.GatePending,
		CreatedAt:  o.now(),
		Payload: domain.GatePayload{
			DiffPreview:    res.Diff,
			Evidence:       []domain.EvidenceRef{{Type: "decision", Summary: "decision phase output"}},
			RiskAssessment: domain.RiskMedium,
		},
	}
	if err := o.store.CreateGate(ctx, gate); err != nil {
		return err
	}
	return o.appendEvent(ctx, wi.ID, domain.EventGateCreated, domain.ActorSystem, map[string]any{
		"gate_id": string(gate.ID),
	})
}

// grantAuthorization records IMPLEMENTATION=AUTHORIZED and advances to
// implementation. Authorization is a recorded condition, not a phase.
func (o *Orchestrator) grantAuthorization(ctx context.Context, wi *domain.WorkItem, by string) error {
	gate, err := o.store.GetGateByWorkItem(ctx, wi.ID)
	if err != nil {
		return err
	}
	now := o.now()
	gate.Approvals = append(gate.Approvals, domain.Approval{
		User:     by,
		Decision: domain.ApprovalApprove,
		At:       now,
		Authorization: &domain.AuthorizationRecord{
			Granted:   true,
			GrantedBy: by,
			GrantedAt: now,
		},
	})
	gate.Status = domain.GateApproved
	gate.ResolvedAt = &now
	if err := o.store.UpdateGate(ctx, gate); err != nil {
		return err
	}
	_ = o.appendEvent(ctx, wi.ID, domain.EventAuthorizationGranted, domain.ActorHuman, map[string]any{"by": by})
	_ = o.appendEvent(ctx, wi.ID, domain.EventGateApproved, domain.ActorHuman, map[string]any{"gate_id": string(gate.ID)})
	return o.updatePhase(ctx, wi, domain.PhaseImplementation)
}

// markFailed transitions the WorkItem to failed, retaining its worktree.
func (o *Orchestrator) markFailed(ctx context.Context, wi *domain.WorkItem, cause error) error {
	prev := wi.CurrentPhase
	wi.CurrentPhase = domain.PhaseFailed
	wi.Status = domain.PhaseFailed
	wi.UpdatedAt = o.now()
	if err := o.store.UpdateWorkItem(ctx, wi); err != nil {
		return err
	}
	_ = o.appendEvent(ctx, wi.ID, domain.EventPhaseFailed, domain.ActorSystem, map[string]any{
		"phase": string(prev),
		"error": cause.Error(),
	})
	return cause
}

// cleanupWorktree removes the worktree after a successful completion.
// Failed/blocked WorkItems retain their worktree for debugging.
func (o *Orchestrator) cleanupWorktree(ctx context.Context, wi *domain.WorkItem) {
	if err := o.worktrees.Cleanup(ctx, string(wi.ID)); err != nil {
		return
	}
	_ = o.appendEvent(ctx, wi.ID, domain.EventWorktreeCleaned, domain.ActorSystem, map[string]any{
		"path": wi.WorktreePath,
	})
}
