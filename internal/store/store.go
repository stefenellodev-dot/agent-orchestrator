package store

import (
	"context"
	"errors"

	"github.com/stefenello/agent-orchestrator/internal/domain"
)

// ErrNotFound is returned when a requested entity does not exist.
var ErrNotFound = errors.New("not found")

// ErrProjectBusy is returned when a project already has an active WorkItem
// and the configured concurrency limit forbids another.
var ErrProjectBusy = errors.New("project already has an active work item")

// Store is the persistence boundary. The MVP ships an in-memory implementation
// and a PostgreSQL implementation behind this same interface.
type Store interface {
	CreateWorkItem(ctx context.Context, wi *domain.WorkItem) error
	GetWorkItem(ctx context.Context, id domain.WorkItemID) (*domain.WorkItem, error)
	UpdateWorkItem(ctx context.Context, wi *domain.WorkItem) error
	ListWorkItems(ctx context.Context, project string) ([]*domain.WorkItem, error)

	CreateSession(ctx context.Context, s *domain.Session) error
	GetSession(ctx context.Context, id domain.SessionID) (*domain.Session, error)
	UpdateSession(ctx context.Context, s *domain.Session) error
	ListSessions(ctx context.Context, workItemID domain.WorkItemID) ([]*domain.Session, error)

	CreateGate(ctx context.Context, g *domain.Gate) error
	GetGate(ctx context.Context, id domain.GateID) (*domain.Gate, error)
	GetGateByWorkItem(ctx context.Context, workItemID domain.WorkItemID) (*domain.Gate, error)
	UpdateGate(ctx context.Context, g *domain.Gate) error

	AppendEvent(ctx context.Context, e *domain.Event) error
	ListEvents(ctx context.Context, workItemID domain.WorkItemID) ([]*domain.Event, error)
}
