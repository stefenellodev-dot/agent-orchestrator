package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/store"
)

var (
	// ErrInvalidInput indicates the caller supplied an incomplete request.
	ErrInvalidInput = errors.New("invalid input")
	// ErrNotFound indicates the requested entity does not exist.
	ErrNotFound = errors.New("not found")
	// ErrAuthorizationRequired is returned when a transition to implementation
	// is attempted without a recorded IMPLEMENTATION=AUTHORIZED condition.
	ErrAuthorizationRequired = errors.New("implementation not authorized")
	// ErrInvalidTransition is returned for illegal state transitions.
	ErrInvalidTransition = errors.New("invalid phase transition")
	// ErrProjectBusy is returned when the project concurrency limit is reached.
	ErrProjectBusy = errors.New("project already has an active work item")
)

// CreateWorkItemInput is the caller-supplied description of new work.
type CreateWorkItemInput struct {
	Project     string
	Title       string
	Description string
	Priority    domain.Priority
	BaseBranch  string
	RepoPath    string
	Assignee    string
	Metadata    domain.Metadata
}

// Orchestrator holds the WorkItem state machine and coordinates the store,
// worktree manager, and OpenCode adapter.
type Orchestrator struct {
	store     store.Store
	worktrees WorktreeManager
	adapter   domain.OpenCodeAdapter
	now       func() time.Time

	// autoDrive starts workflow execution in the background when a WorkItem is
	// created. Off by default so unit tests can drive explicitly.
	autoDrive bool
	// autoApprove bypasses the human gate. Test-only; MUST stay false in
	// production, where the gate pauses the workflow.
	autoApprove  bool
	agents       map[domain.Phase]string
	model        string
	phaseTimeout time.Duration
	collector    *EvidenceCollector
	projects     map[string]ProjectConfig

	// inflight tracks WorkItems currently being driven in this process, so a
	// WorkItem is never processed by two drivers at once (single-driver
	// discipline; also makes startup reconciliation safe/idempotent).
	inflightMu sync.Mutex
	inflight   map[domain.WorkItemID]bool

	// maxActivePerProject is the configurable per-project concurrency limit
	// (R5). createMu serialises creation so the limit is enforced race-free.
	maxActivePerProject int
	createMu            sync.Mutex
}

// ProjectConfig is the orchestrator-owned definition of a project. Consumer
// repositories need no changes; paths and validation commands live here.
type ProjectConfig struct {
	Name       string            `json:"name"`
	RepoPath   string            `json:"repo_path"`
	BaseBranch string            `json:"base_branch"`
	Validation ProjectValidation `json:"validation"`
}

// ProjectList returns the registered projects, ordered by name.
func (o *Orchestrator) ProjectList() []ProjectConfig {
	out := make([]ProjectConfig, 0, len(o.projects))
	for _, pc := range o.projects {
		out = append(out, pc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// New builds an Orchestrator. adapter may be nil; phases that require OpenCode
// will then fail loudly rather than silently no-op.
func New(s store.Store, wm WorktreeManager, adapter domain.OpenCodeAdapter) *Orchestrator {
	return &Orchestrator{
		store:     s,
		worktrees: wm,
		adapter:   adapter,
		now:       func() time.Time { return time.Now().UTC() },
		agents: map[domain.Phase]string{
			domain.PhaseDiscovery:      "explore",
			domain.PhaseDecision:       "plan",
			domain.PhaseImplementation: "build",
			domain.PhaseValidation:     "build",
		},
		phaseTimeout: 10 * time.Minute,
		collector:    NewEvidenceCollector(),
		projects:     map[string]ProjectConfig{},
		inflight:     map[domain.WorkItemID]bool{},

		maxActivePerProject: 1,
	}
}

// SetMaxActivePerProject sets the configurable per-project concurrency limit
// (R5). Values < 1 are treated as 1.
func (o *Orchestrator) SetMaxActivePerProject(n int) {
	if n < 1 {
		n = 1
	}
	o.maxActivePerProject = n
}

// activeCount returns the number of non-terminal WorkItems for a project.
func (o *Orchestrator) activeCount(ctx context.Context, project string) (int, error) {
	items, err := o.store.ListWorkItems(ctx, project)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, wi := range items {
		if !wi.CurrentPhase.IsTerminal() {
			n++
		}
	}
	return n, nil
}

// RegisterProject registers (or replaces) a project definition.
func (o *Orchestrator) RegisterProject(pc ProjectConfig) {
	o.projects[pc.Name] = pc
}

// SetProjectValidation registers the objective validation commands for a
// project, preserving any existing path configuration.
func (o *Orchestrator) SetProjectValidation(project string, pv ProjectValidation) {
	pc := o.projects[project]
	pc.Name = project
	pc.Validation = pv
	o.projects[project] = pc
}

func (o *Orchestrator) validationFor(project string) ProjectValidation {
	return o.projects[project].Validation
}

// SetModel sets the explicit OpenCode provider/model used for all phases.
func (o *Orchestrator) SetModel(model string) { o.model = model }

// SetAutoDrive enables background workflow execution on WorkItem creation.
func (o *Orchestrator) SetAutoDrive(v bool) { o.autoDrive = v }

// SetAutoApprove bypasses the human gate. Test-only.
func (o *Orchestrator) SetAutoApprove(v bool) { o.autoApprove = v }

// SetAgents overrides the per-phase OpenCode agent names.
func (o *Orchestrator) SetAgents(discovery, decision, implementation, validation string) {
	if discovery != "" {
		o.agents[domain.PhaseDiscovery] = discovery
	}
	if decision != "" {
		o.agents[domain.PhaseDecision] = decision
	}
	if implementation != "" {
		o.agents[domain.PhaseImplementation] = implementation
	}
	if validation != "" {
		o.agents[domain.PhaseValidation] = validation
	}
}

// SetPhaseTimeout overrides the per-phase execution timeout.
func (o *Orchestrator) SetPhaseTimeout(d time.Duration) {
	if d > 0 {
		o.phaseTimeout = d
	}
}

func (o *Orchestrator) agentFor(phase domain.Phase) string { return o.agents[phase] }

// CreateWorkItem creates a WorkItem, provisions exactly one worktree, and
// starts the Discovery phase. It emits workitem.created and phase.started.
func (o *Orchestrator) CreateWorkItem(ctx context.Context, in CreateWorkItemInput) (*domain.WorkItem, error) {
	if in.Project == "" || in.Title == "" {
		return nil, fmt.Errorf("%w: project and title are required", ErrInvalidInput)
	}
	// Resolve repository location from registered project config when the
	// caller does not supply it explicitly.
	if pc, ok := o.projects[in.Project]; ok {
		if in.RepoPath == "" {
			in.RepoPath = pc.RepoPath
		}
		if in.BaseBranch == "" {
			in.BaseBranch = pc.BaseBranch
		}
	}
	if in.BaseBranch == "" {
		in.BaseBranch = "main"
	}
	if in.Priority == "" {
		in.Priority = domain.PriorityMedium
	}
	if in.Metadata == nil {
		in.Metadata = domain.Metadata{}
	}

	// Enforce the configurable per-project concurrency limit (R5). Creation is
	// serialised in-process so concurrent requests cannot overshoot the limit.
	o.createMu.Lock()
	defer o.createMu.Unlock()
	active, err := o.activeCount(ctx, in.Project)
	if err != nil {
		return nil, err
	}
	if active >= o.maxActivePerProject {
		return nil, ErrProjectBusy
	}

	now := o.now()
	wi := &domain.WorkItem{
		ID:           domain.NewWorkItemID(),
		Project:      in.Project,
		Title:        in.Title,
		Description:  in.Description,
		Priority:     in.Priority,
		Status:       domain.PhaseDiscovery,
		CurrentPhase: domain.PhaseDiscovery,
		BaseBranch:   in.BaseBranch,
		Assignee:     in.Assignee,
		Metadata:     in.Metadata,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	wt, err := o.worktrees.Create(ctx, string(wi.ID), in.RepoPath, in.BaseBranch)
	if err != nil {
		return nil, fmt.Errorf("provision worktree: %w", err)
	}
	wi.WorktreePath = wt.Path
	wi.BaseCommitSHA = wt.BaseCommitSHA

	if err := o.store.CreateWorkItem(ctx, wi); err != nil {
		// The store rejected the WorkItem (e.g. project concurrency limit);
		// reclaim the worktree we provisioned so it is not orphaned.
		_ = o.worktrees.Cleanup(ctx, string(wi.ID))
		if errors.Is(err, store.ErrProjectBusy) {
			return nil, ErrProjectBusy
		}
		return nil, err
	}
	if err := o.appendEvent(ctx, wi.ID, domain.EventWorkItemCreated, domain.ActorHuman, map[string]any{
		"project": wi.Project,
		"title":   wi.Title,
	}); err != nil {
		return nil, err
	}
	if err := o.appendEvent(ctx, wi.ID, domain.EventWorktreeCreated, domain.ActorSystem, map[string]any{
		"path": wi.WorktreePath,
	}); err != nil {
		return nil, err
	}
	if err := o.appendEvent(ctx, wi.ID, domain.EventPhaseStarted, domain.ActorSystem, map[string]any{
		"phase": string(domain.PhaseDiscovery),
	}); err != nil {
		return nil, err
	}

	if o.autoDrive && o.adapter != nil {
		go func(id domain.WorkItemID) {
			_ = o.Drive(context.Background(), id)
		}(wi.ID)
	}
	return wi, nil
}

// GetWorkItem returns a WorkItem or ErrNotFound.
func (o *Orchestrator) GetWorkItem(ctx context.Context, id domain.WorkItemID) (*domain.WorkItem, error) {
	wi, err := o.store.GetWorkItem(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return wi, nil
}

// ListWorkItems returns WorkItems, optionally filtered by project.
func (o *Orchestrator) ListWorkItems(ctx context.Context, project string) ([]*domain.WorkItem, error) {
	return o.store.ListWorkItems(ctx, project)
}

// ListEvents returns the ordered audit trail for a WorkItem.
func (o *Orchestrator) ListEvents(ctx context.Context, id domain.WorkItemID) ([]*domain.Event, error) {
	if _, err := o.GetWorkItem(ctx, id); err != nil {
		return nil, err
	}
	return o.store.ListEvents(ctx, id)
}

// ListSessions returns the OpenCode executions recorded for a WorkItem.
func (o *Orchestrator) ListSessions(ctx context.Context, id domain.WorkItemID) ([]*domain.Session, error) {
	if _, err := o.GetWorkItem(ctx, id); err != nil {
		return nil, err
	}
	return o.store.ListSessions(ctx, id)
}

func (o *Orchestrator) appendEvent(ctx context.Context, id domain.WorkItemID, t domain.EventType, actor domain.EventActor, payload map[string]any) error {
	return o.store.AppendEvent(ctx, &domain.Event{
		ID:         domain.NewEventID(),
		WorkItemID: id,
		Type:       t,
		Payload:    payload,
		Actor:      actor,
		At:         o.now(),
	})
}

// updatePhase persists a WorkItem phase change and emits phase events.
func (o *Orchestrator) updatePhase(ctx context.Context, wi *domain.WorkItem, next domain.Phase) error {
	prev := wi.CurrentPhase
	wi.CurrentPhase = next
	wi.Status = next
	wi.UpdatedAt = o.now()
	if err := o.store.UpdateWorkItem(ctx, wi); err != nil {
		return err
	}
	if err := o.appendEvent(ctx, wi.ID, domain.EventPhaseCompleted, domain.ActorSystem, map[string]any{
		"phase": string(prev),
	}); err != nil {
		return err
	}
	return o.appendEvent(ctx, wi.ID, domain.EventPhaseStarted, domain.ActorSystem, map[string]any{
		"phase": string(next),
	})
}
