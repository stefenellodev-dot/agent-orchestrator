package domain

import (
	"time"

	"github.com/oklog/ulid/v2"
)

// WorkItemID is a ULID-encoded identifier.
type WorkItemID string

// SessionID is a ULID-encoded identifier.
type SessionID string

// GateID is a ULID-encoded identifier.
type GateID string

// EventID is a ULID-encoded identifier.
type EventID string

func newID() string { return ulid.Make().String() }

func NewWorkItemID() WorkItemID { return WorkItemID(newID()) }
func NewSessionID() SessionID   { return SessionID(newID()) }
func NewGateID() GateID         { return GateID(newID()) }
func NewEventID() EventID       { return EventID(newID()) }

// Phase is a state in the WorkItem state machine.
type Phase string

const (
	PhaseDiscovery        Phase = "discovery"
	PhaseDecision         Phase = "decision"
	PhaseAwaitingApproval Phase = "awaiting_approval"
	PhaseImplementation   Phase = "implementation"
	PhaseValidation       Phase = "validation"
	PhaseComplete         Phase = "complete"
	PhaseBlocked          Phase = "blocked"
	PhaseFailed           Phase = "failed"
)

// phaseOrder is the linear happy path. Blocked and Failed are terminal
// off-ramps and are intentionally absent from the linear order.
var phaseOrder = []Phase{
	PhaseDiscovery,
	PhaseDecision,
	PhaseAwaitingApproval,
	PhaseImplementation,
	PhaseValidation,
	PhaseComplete,
}

// IsTerminal reports whether no further automatic transition is possible.
func (p Phase) IsTerminal() bool {
	return p == PhaseComplete || p == PhaseBlocked || p == PhaseFailed
}

// RequiresOpenCodeRun reports whether the phase is executed by invoking OpenCode.
func (p Phase) RequiresOpenCodeRun() bool {
	switch p {
	case PhaseDiscovery, PhaseDecision, PhaseImplementation, PhaseValidation:
		return true
	default:
		return false
	}
}

// Next returns the next phase on the linear happy path. Terminal or unknown
// phases return themselves.
func (p Phase) Next() Phase {
	for i, ph := range phaseOrder {
		if ph == p && i+1 < len(phaseOrder) {
			return phaseOrder[i+1]
		}
	}
	return p
}

type Priority string

const (
	PriorityLow      Priority = "low"
	PriorityMedium   Priority = "medium"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

// WorkItem is the unit of work flowing through the pipeline.
type WorkItem struct {
	ID           WorkItemID `json:"id"`
	Project      string     `json:"project"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Priority     Priority   `json:"priority"`
	Status       Phase      `json:"status"`
	CurrentPhase Phase      `json:"current_phase"`
	// WorktreePath is provisioned once and reused for every phase.
	WorktreePath string `json:"worktree_path"`
	BaseBranch   string `json:"base_branch"`
	// BaseCommitSHA is the immutable commit the WorkItem was branched from.
	// Evidence is always computed against this SHA, never against a moving
	// branch reference.
	BaseCommitSHA string    `json:"base_commit_sha,omitempty"`
	Assignee      string    `json:"assignee,omitempty"`
	Metadata      Metadata  `json:"metadata"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Metadata is free-form, non-authoritative data. It must never drive state.
type Metadata map[string]any

type SessionStatus string

const (
	SessionPending   SessionStatus = "pending"
	SessionRunning   SessionStatus = "running"
	SessionCompleted SessionStatus = "completed"
	SessionFailed    SessionStatus = "failed"
)

// Session records one OpenCode execution for one phase of one WorkItem.
type Session struct {
	ID              SessionID      `json:"id"`
	WorkItemID      WorkItemID     `json:"work_item_id"`
	Phase           Phase          `json:"phase"`
	OpenCodeSession string         `json:"opencode_session,omitempty"`
	Status          SessionStatus  `json:"status"`
	Agent           string         `json:"agent"`
	Prompt          string         `json:"prompt"`
	Output          *SessionOutput `json:"output,omitempty"`
	Error           string         `json:"error,omitempty"`
	ExitCode        int            `json:"exit_code"`
	StartedAt       time.Time      `json:"started_at"`
	CompletedAt     *time.Time     `json:"completed_at,omitempty"`
}

type GateStatus string

const (
	GatePending          GateStatus = "pending"
	GateApproved         GateStatus = "approved"
	GateRejected         GateStatus = "rejected"
	GateChangesRequested GateStatus = "changes_requested"
)

type ApprovalDecision string

const (
	ApprovalApprove        ApprovalDecision = "approve"
	ApprovalReject         ApprovalDecision = "reject"
	ApprovalRequestChanges ApprovalDecision = "request_changes"
)

// AuthorizationRecord captures the IMPLEMENTATION=AUTHORIZED condition.
// Authorization is a recorded condition, not a phase.
type AuthorizationRecord struct {
	Granted           bool      `json:"granted"`
	GrantedBy         string    `json:"granted_by"`
	GrantedAt         time.Time `json:"granted_at"`
	ApprovedCommitSHA string    `json:"approved_commit_sha,omitempty"`
}

type Approval struct {
	User          string               `json:"user"`
	Decision      ApprovalDecision     `json:"decision"`
	Comment       string               `json:"comment,omitempty"`
	At            time.Time            `json:"at"`
	Authorization *AuthorizationRecord `json:"authorization,omitempty"`
}

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

type EvidenceRef struct {
	SessionID SessionID `json:"session_id"`
	Type      string    `json:"type"`
	Summary   string    `json:"summary"`
}

// Plan is the agent-proposed implementation plan shown to the human.
type Plan struct {
	Steps        []PlanStep `json:"steps"`
	TestStrategy string     `json:"test_strategy,omitempty"`
	// Confidence is agent self-assessment metadata only. It NEVER determines
	// whether a WorkItem reaches COMPLETE. Objective evidence does.
	Confidence float64 `json:"confidence,omitempty"`
}

type PlanStep struct {
	Description string   `json:"description"`
	Files       []string `json:"files,omitempty"`
	Commands    []string `json:"commands,omitempty"`
}

// GatePayload is everything the human reviews before authorizing.
type GatePayload struct {
	Plan           Plan          `json:"plan"`
	DiffPreview    string        `json:"diff_preview,omitempty"`
	Evidence       []EvidenceRef `json:"evidence"`
	RiskAssessment RiskLevel     `json:"risk_assessment"`
}

type Gate struct {
	ID                GateID      `json:"id"`
	WorkItemID        WorkItemID  `json:"work_item_id"`
	Phase             Phase       `json:"phase"`
	RequiredApprovers []string    `json:"required_approvers"`
	Approvals         []Approval  `json:"approvals"`
	Status            GateStatus  `json:"status"`
	Payload           GatePayload `json:"payload"`
	CreatedAt         time.Time   `json:"created_at"`
	ResolvedAt        *time.Time  `json:"resolved_at,omitempty"`
}

type EventActor string

const (
	ActorSystem    EventActor = "system"
	ActorOpenCode  EventActor = "opencode"
	ActorHuman     EventActor = "human"
	ActorScheduler EventActor = "scheduler"
)

type EventType string

const (
	EventWorkItemCreated      EventType = "workitem.created"
	EventPhaseStarted         EventType = "phase.started"
	EventPhaseCompleted       EventType = "phase.completed"
	EventPhaseFailed          EventType = "phase.failed"
	EventGateCreated          EventType = "gate.created"
	EventGateApproved         EventType = "gate.approved"
	EventGateRejected         EventType = "gate.rejected"
	EventGateChangesRequested EventType = "gate.changes_requested"
	EventSessionStarted       EventType = "session.started"
	EventSessionCompleted     EventType = "session.completed"
	EventSessionFailed        EventType = "session.failed"
	EventWorktreeCreated      EventType = "worktree.created"
	EventWorktreeCommitted    EventType = "worktree.committed"
	EventWorktreeCleaned      EventType = "worktree.cleaned"
	EventAuthorizationGranted EventType = "authorization.granted"
	EventAuthorizationDenied  EventType = "authorization.denied"
	EventWorkItemRetried      EventType = "workitem.retried"
)

// Event is an immutable audit-log entry. Persistence (Postgres) is the audit
// trail for the MVP; no message broker or event-sourcing infrastructure.
type Event struct {
	ID         EventID        `json:"id"`
	WorkItemID WorkItemID     `json:"work_item_id"`
	Type       EventType      `json:"type"`
	Payload    map[string]any `json:"payload"`
	Actor      EventActor     `json:"actor"`
	At         time.Time      `json:"at"`
}
