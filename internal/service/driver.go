package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/store"
)

// Drive runs the WorkItem workflow until it reaches a terminal phase or stops
// at the human gate. It enforces a single-driver discipline: concurrent calls
// for the same WorkItem are ignored (the second returns immediately), so the
// auto-drive path and startup reconciliation can never process one WorkItem
// twice. It is safe to call multiple times.
func (o *Orchestrator) Drive(ctx context.Context, id domain.WorkItemID) error {
	if !o.acquire(id) {
		return nil // already being driven by this process
	}
	defer o.release(id)
	return o.driveLoop(ctx, id)
}

func (o *Orchestrator) acquire(id domain.WorkItemID) bool {
	o.inflightMu.Lock()
	defer o.inflightMu.Unlock()
	if o.inflight[id] {
		return false
	}
	o.inflight[id] = true
	return true
}

func (o *Orchestrator) release(id domain.WorkItemID) {
	o.inflightMu.Lock()
	delete(o.inflight, id)
	o.inflightMu.Unlock()
}

// isInflight reports whether a driver for the WorkItem is running in this
// process. Startup reconciliation uses it to distinguish a legitimate active
// execution (owned here, must not be interrupted) from an orphaned one.
func (o *Orchestrator) isInflight(id domain.WorkItemID) bool {
	o.inflightMu.Lock()
	defer o.inflightMu.Unlock()
	return o.inflight[id]
}

func (o *Orchestrator) driveLoop(ctx context.Context, id domain.WorkItemID) error {
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

		// R6 execution guards: project/runtime/worktree validity before
		// Implementation. A rejection blocks the WorkItem (no auto-correction).
		if phase == domain.PhaseImplementation {
			if err := o.executionGuards(ctx, wi); err != nil {
				return o.markBlocked(ctx, wi, err)
			}
		}

		res, err := o.executePhase(ctx, wi)
		if err != nil {
			return o.markFailed(ctx, wi, err)
		}

		if phase == domain.PhaseValidation {
			out, err := o.collectValidationEvidence(ctx, wi)
			if err != nil {
				return o.markFailed(ctx, wi, err)
			}
			if ok, reason := validationPassed(out, res.ExitCode); !ok {
				return o.markFailed(ctx, wi, fmt.Errorf("validation failed: %s", reason))
			}
			// R6 completion guards: protected paths + mandatory validation evidence.
			if err := o.completionGuards(ctx, wi, out); err != nil {
				return o.markFailed(ctx, wi, err)
			}
		} else if res.ExitCode != 0 {
			// A failed Discovery/Decision/Implementation must halt the workflow
			// rather than advance to the gate.
			return o.markFailed(ctx, wi, fmt.Errorf("%s phase exited %d", phase, res.ExitCode))
		}

		if phase == domain.PhaseImplementation {
			o.commitImplementation(ctx, wi)
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
	prompt := PromptFor(wi, wi.CurrentPhase, o.approvedPlanFor(ctx, wi, wi.CurrentPhase))

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
		WorktreePath:  wi.WorktreePath,
		BaseBranch:    wi.BaseBranch,
		BaseCommitSHA: wi.BaseCommitSHA,
		Phase:         wi.CurrentPhase,
		Prompt:        prompt,
		Agent:         agent,
		Model:         o.model,
		Timeout:       o.phaseTimeout,
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
		AgentText:        res.AgentText,
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
	switch {
	case res.TimedOut:
		sess.Status = domain.SessionFailed
		sess.Error = "runtime timeout"
	case res.Canceled:
		sess.Status = domain.SessionFailed
		sess.Error = "runtime canceled"
	case res.ExitCode == 0:
		sess.Status = domain.SessionCompleted
	default:
		sess.Status = domain.SessionFailed
		sess.Error = strings.TrimSpace(res.Stderr)
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

// commitImplementation guarantees the implementation produces a commit and
// refreshes the implementation session's git evidence to match it. This keeps
// the commit SHA and diff reproducible even if the agent forgot to commit.
func (o *Orchestrator) commitImplementation(ctx context.Context, wi *domain.WorkItem) {
	message := fmt.Sprintf("orchestrator: implement %q (%s)", wi.Title, wi.ID)
	committed, err := gitCommitAll(ctx, wi.WorktreePath, message)
	if err != nil || !committed {
		return
	}
	head, _ := revParse(ctx, wi.WorktreePath)
	_ = o.appendEvent(ctx, wi.ID, domain.EventWorktreeCommitted, domain.ActorSystem, map[string]any{
		"commit_sha": head,
	})

	sessions, err := o.store.ListSessions(ctx, wi.ID)
	if err != nil {
		return
	}
	for i := len(sessions) - 1; i >= 0; i-- {
		if sessions[i].Phase != domain.PhaseImplementation {
			continue
		}
		sess := sessions[i]
		if sess.Output == nil {
			sess.Output = &domain.SessionOutput{}
		}
		sess.Output.CommitSHA = head
		if base := sess.Output.BaseCommitSHA; base != "" && head != "" && base != head {
			if diff, err := runGitCommand(ctx, wi.WorktreePath, "diff", base+".."+head); err == nil {
				sess.Output.Diff = diff
			}
			if stat, err := runGitCommand(ctx, wi.WorktreePath, "diff", "--stat", base+".."+head); err == nil {
				sess.Output.DiffStat = stat
			}
		}
		_ = o.store.UpdateSession(ctx, sess)
		return
	}
}

// approvedPlanFor returns the human-approved plan text (Discovery + Decision)
// for the Implementation phase. It is empty for every other phase.
func (o *Orchestrator) approvedPlanFor(ctx context.Context, wi *domain.WorkItem, phase domain.Phase) string {
	if phase != domain.PhaseImplementation {
		return ""
	}
	sessions, err := o.store.ListSessions(ctx, wi.ID)
	if err != nil {
		return ""
	}
	var discovery, decision string
	for _, s := range sessions {
		switch s.Phase {
		case domain.PhaseDiscovery:
			discovery = phaseText(s)
		case domain.PhaseDecision:
			decision = phaseText(s)
		}
	}
	var b strings.Builder
	if strings.TrimSpace(discovery) != "" {
		b.WriteString("## Discovery\n")
		b.WriteString(discovery)
		b.WriteString("\n\n")
	}
	if strings.TrimSpace(decision) != "" {
		b.WriteString("## Decision\n")
		b.WriteString(decision)
	}
	return b.String()
}

// phaseText returns a session's agent text, recovering it from stored raw
// stdout for legacy sessions that predate AgentText extraction.
func phaseText(s *domain.Session) string {
	if s == nil || s.Output == nil {
		return ""
	}
	if strings.TrimSpace(s.Output.AgentText) != "" {
		return s.Output.AgentText
	}
	return extractAgentText(s.Output.ValidationOutput)
}

// createGate opens (or re-opens) the human approval gate after Decision
// completes. On re-planning the existing gate is reset rather than duplicated.
func (o *Orchestrator) createGate(ctx context.Context, wi *domain.WorkItem, res *domain.RunResult) error {
	payload := domain.GatePayload{
		DiffPreview:    res.Diff,
		Evidence:       []domain.EvidenceRef{{Type: "decision", Summary: "decision phase output"}},
		RiskAssessment: domain.RiskMedium,
	}

	existing, err := o.store.GetGateByWorkItem(ctx, wi.ID)
	switch {
	case err == nil:
		existing.Status = domain.GatePending
		existing.Payload = payload
		existing.Approvals = nil
		existing.ResolvedAt = nil
		if err := o.store.UpdateGate(ctx, existing); err != nil {
			return err
		}
		if err := o.appendEvent(ctx, wi.ID, domain.EventGateCreated, domain.ActorSystem, map[string]any{
			"gate_id":  string(existing.ID),
			"reopened": true,
		}); err != nil {
			return err
		}
	case errors.Is(err, store.ErrNotFound):
		gate := &domain.Gate{
			ID:         domain.NewGateID(),
			WorkItemID: wi.ID,
			Phase:      domain.PhaseAwaitingApproval,
			Status:     domain.GatePending,
			CreatedAt:  o.now(),
			Payload:    payload,
		}
		if err := o.store.CreateGate(ctx, gate); err != nil {
			return err
		}
		if err := o.appendEvent(ctx, wi.ID, domain.EventGateCreated, domain.ActorSystem, map[string]any{
			"gate_id": string(gate.ID),
		}); err != nil {
			return err
		}
	default:
		return err
	}
	return nil
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
	return o.AdvanceToImplementation(ctx, wi.ID)
}

// collectValidationEvidence runs the orchestrator-owned validation commands and
// persists their objective results onto the validation Session.
func (o *Orchestrator) collectValidationEvidence(ctx context.Context, wi *domain.WorkItem) (*domain.SessionOutput, error) {
	pv := o.validationFor(wi.Project)
	collector := o.collector
	if collector == nil {
		collector = NewEvidenceCollector()
	}
	out, err := collector.Collect(ctx, wi.WorktreePath, wi.BaseBranch, wi.BaseCommitSHA, pv)
	if err != nil {
		return nil, err
	}

	sessions, err := o.store.ListSessions(ctx, wi.ID)
	if err == nil {
		for i := len(sessions) - 1; i >= 0; i-- {
			if sessions[i].Phase != domain.PhaseValidation {
				continue
			}
			sess := sessions[i]
			if sess.Output == nil {
				sess.Output = &domain.SessionOutput{}
			}
			sess.Output.BaseCommitSHA = out.BaseCommitSHA
			sess.Output.CommitSHA = out.CommitSHA
			sess.Output.Diff = out.Diff
			sess.Output.DiffStat = out.DiffStat
			sess.Output.TestCommands = out.TestCommands
			sess.Output.LintCommands = out.LintCommands
			sess.Output.TypecheckCommands = out.TypecheckCommands
			_ = o.store.UpdateSession(ctx, sess)
			break
		}
	}
	return out, nil
}

// validationPassed reports whether the WorkItem may be marked complete. Only
// objective command exit codes decide; agent confidence is never consulted.
func validationPassed(out *domain.SessionOutput, openCodeExit int) (bool, string) {
	if out == nil {
		out = &domain.SessionOutput{}
	}
	commands := make([]domain.CommandResult, 0, len(out.TestCommands)+len(out.LintCommands)+len(out.TypecheckCommands))
	commands = append(commands, out.TestCommands...)
	commands = append(commands, out.LintCommands...)
	commands = append(commands, out.TypecheckCommands...)

	if len(commands) == 0 {
		// No objective commands configured: fall back to the OpenCode exit code.
		if openCodeExit != 0 {
			return false, fmt.Sprintf("opencode validation exited %d", openCodeExit)
		}
		return true, ""
	}
	for _, c := range commands {
		if c.ExitCode != 0 {
			return false, fmt.Sprintf("command %q exited %d", c.Command, c.ExitCode)
		}
	}
	return true, ""
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

// markBlocked halts a WorkItem at a policy guardrail. Unlike markFailed, the
// worktree is retained and the phase advanced to blocked for human triage.
func (o *Orchestrator) markBlocked(ctx context.Context, wi *domain.WorkItem, cause error) error {
	prev := wi.CurrentPhase
	wi.CurrentPhase = domain.PhaseBlocked
	wi.Status = domain.PhaseBlocked
	wi.UpdatedAt = o.now()
	if err := o.store.UpdateWorkItem(ctx, wi); err != nil {
		return err
	}
	_ = o.appendEvent(ctx, wi.ID, domain.EventPhaseBlocked, domain.ActorSystem, map[string]any{
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
