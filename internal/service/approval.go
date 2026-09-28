package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/store"
)

// ApprovalInput carries a human decision on a gate.
type ApprovalInput struct {
	User    string
	Comment string
}

// GetGate returns the gate for a WorkItem.
func (o *Orchestrator) GetGate(ctx context.Context, workItemID domain.WorkItemID) (*domain.Gate, error) {
	gate, err := o.store.GetGateByWorkItem(ctx, workItemID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return gate, nil
}

// Approve records IMPLEMENTATION=AUTHORIZED and advances to implementation.
func (o *Orchestrator) Approve(ctx context.Context, id domain.WorkItemID, in ApprovalInput) error {
	_, gate, err := o.loadPendingGate(ctx, id)
	if err != nil {
		return err
	}
	now := o.now()
	gate.Approvals = append(gate.Approvals, domain.Approval{
		User:     in.User,
		Decision: domain.ApprovalApprove,
		Comment:  in.Comment,
		At:       now,
		Authorization: &domain.AuthorizationRecord{
			Granted:   true,
			GrantedBy: in.User,
			GrantedAt: now,
		},
	})
	gate.Status = domain.GateApproved
	gate.ResolvedAt = &now
	if err := o.store.UpdateGate(ctx, gate); err != nil {
		return err
	}
	_ = o.appendEvent(ctx, id, domain.EventAuthorizationGranted, domain.ActorHuman, map[string]any{"by": in.User})
	_ = o.appendEvent(ctx, id, domain.EventGateApproved, domain.ActorHuman, map[string]any{"gate_id": string(gate.ID)})

	if err := o.AdvanceToImplementation(ctx, id); err != nil {
		return err
	}
	o.resumeDrive(id)
	return nil
}

// RequestChanges sends the WorkItem back to Decision for re-planning.
func (o *Orchestrator) RequestChanges(ctx context.Context, id domain.WorkItemID, in ApprovalInput) error {
	wi, gate, err := o.loadPendingGate(ctx, id)
	if err != nil {
		return err
	}
	now := o.now()
	gate.Approvals = append(gate.Approvals, domain.Approval{
		User: in.User, Decision: domain.ApprovalRequestChanges, Comment: in.Comment, At: now,
	})
	gate.Status = domain.GateChangesRequested
	gate.ResolvedAt = &now
	if err := o.store.UpdateGate(ctx, gate); err != nil {
		return err
	}
	_ = o.appendEvent(ctx, id, domain.EventGateChangesRequested, domain.ActorHuman, map[string]any{
		"by": in.User, "comment": in.Comment,
	})
	if err := o.updatePhase(ctx, wi, domain.PhaseDecision); err != nil {
		return err
	}
	o.resumeDrive(id)
	return nil
}

// Reject blocks the WorkItem for human triage.
func (o *Orchestrator) Reject(ctx context.Context, id domain.WorkItemID, in ApprovalInput) error {
	wi, gate, err := o.loadPendingGate(ctx, id)
	if err != nil {
		return err
	}
	now := o.now()
	gate.Approvals = append(gate.Approvals, domain.Approval{
		User: in.User, Decision: domain.ApprovalReject, Comment: in.Comment, At: now,
	})
	gate.Status = domain.GateRejected
	gate.ResolvedAt = &now
	if err := o.store.UpdateGate(ctx, gate); err != nil {
		return err
	}
	_ = o.appendEvent(ctx, id, domain.EventGateRejected, domain.ActorHuman, map[string]any{
		"by": in.User, "comment": in.Comment,
	})
	return o.updatePhase(ctx, wi, domain.PhaseBlocked)
}

// AdvanceToImplementation enforces that no WorkItem enters implementation
// without a recorded IMPLEMENTATION=AUTHORIZED condition.
func (o *Orchestrator) AdvanceToImplementation(ctx context.Context, id domain.WorkItemID) error {
	wi, err := o.GetWorkItem(ctx, id)
	if err != nil {
		return err
	}
	if wi.CurrentPhase != domain.PhaseAwaitingApproval {
		return fmt.Errorf("%w: work item is in %q, not awaiting approval", ErrInvalidTransition, wi.CurrentPhase)
	}
	gate, err := o.store.GetGateByWorkItem(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return ErrAuthorizationRequired
	}
	if err != nil {
		return err
	}
	if !hasGrantedAuthorization(gate) {
		return ErrAuthorizationRequired
	}
	return o.updatePhase(ctx, wi, domain.PhaseImplementation)
}

func (o *Orchestrator) loadPendingGate(ctx context.Context, id domain.WorkItemID) (*domain.WorkItem, *domain.Gate, error) {
	wi, err := o.GetWorkItem(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if wi.CurrentPhase != domain.PhaseAwaitingApproval {
		return nil, nil, fmt.Errorf("%w: work item is in %q, not awaiting approval", ErrInvalidTransition, wi.CurrentPhase)
	}
	gate, err := o.store.GetGateByWorkItem(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	if gate.Status == domain.GateApproved || gate.Status == domain.GateRejected {
		return nil, nil, fmt.Errorf("%w: gate already resolved", ErrInvalidTransition)
	}
	return wi, gate, nil
}

func hasGrantedAuthorization(g *domain.Gate) bool {
	for _, a := range g.Approvals {
		if a.Authorization != nil && a.Authorization.Granted {
			return true
		}
	}
	return false
}

func (o *Orchestrator) resumeDrive(id domain.WorkItemID) {
	if o.autoDrive && o.adapter != nil {
		go func() {
			_ = o.Drive(context.Background(), id)
		}()
	}
}
