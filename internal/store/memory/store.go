package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/store"
)

// Store is a thread-safe in-memory implementation of store.Store.
type Store struct {
	mu sync.RWMutex
	// maxActivePerProject enforces the MVP concurrency limit. Defaults to 1,
	// mirroring the PostgreSQL partial unique index.
	maxActivePerProject int
	workItems           map[domain.WorkItemID]*domain.WorkItem
	sessions            map[domain.SessionID]*domain.Session
	gates               map[domain.GateID]*domain.Gate
	gateByWI            map[domain.WorkItemID]domain.GateID
	events              map[domain.WorkItemID][]*domain.Event
}

func New() *Store {
	return &Store{
		maxActivePerProject: 1,
		workItems:           make(map[domain.WorkItemID]*domain.WorkItem),
		sessions:            make(map[domain.SessionID]*domain.Session),
		gates:               make(map[domain.GateID]*domain.Gate),
		gateByWI:            make(map[domain.WorkItemID]domain.GateID),
		events:              make(map[domain.WorkItemID][]*domain.Event),
	}
}

func (s *Store) CreateWorkItem(_ context.Context, wi *domain.WorkItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.workItems[wi.ID]; exists {
		return store.ErrProjectBusy
	}
	active := 0
	for _, existing := range s.workItems {
		if existing.Project == wi.Project && !existing.Status.IsTerminal() {
			active++
		}
	}
	if active >= s.maxActivePerProject {
		return store.ErrProjectBusy
	}
	s.workItems[wi.ID] = cloneWorkItem(wi)
	return nil
}

func (s *Store) GetWorkItem(_ context.Context, id domain.WorkItemID) (*domain.WorkItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	wi, ok := s.workItems[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return cloneWorkItem(wi), nil
}

func (s *Store) UpdateWorkItem(_ context.Context, wi *domain.WorkItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.workItems[wi.ID]; !ok {
		return store.ErrNotFound
	}
	s.workItems[wi.ID] = cloneWorkItem(wi)
	return nil
}

func (s *Store) ListWorkItems(_ context.Context, project string) ([]*domain.WorkItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*domain.WorkItem
	for _, wi := range s.workItems {
		if project == "" || wi.Project == project {
			out = append(out, cloneWorkItem(wi))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) CreateSession(_ context.Context, sess *domain.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.ID] = cloneSession(sess)
	return nil
}

func (s *Store) GetSession(_ context.Context, id domain.SessionID) (*domain.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return cloneSession(sess), nil
}

func (s *Store) UpdateSession(_ context.Context, sess *domain.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[sess.ID]; !ok {
		return store.ErrNotFound
	}
	s.sessions[sess.ID] = cloneSession(sess)
	return nil
}

func (s *Store) ListSessions(_ context.Context, workItemID domain.WorkItemID) ([]*domain.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*domain.Session
	for _, sess := range s.sessions {
		if sess.WorkItemID == workItemID {
			out = append(out, cloneSession(sess))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
	return out, nil
}

func (s *Store) CreateGate(_ context.Context, g *domain.Gate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gates[g.ID] = cloneGate(g)
	s.gateByWI[g.WorkItemID] = g.ID
	return nil
}

func (s *Store) GetGate(_ context.Context, id domain.GateID) (*domain.Gate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.gates[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return cloneGate(g), nil
}

func (s *Store) GetGateByWorkItem(_ context.Context, workItemID domain.WorkItemID) (*domain.Gate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.gateByWI[workItemID]
	if !ok {
		return nil, store.ErrNotFound
	}
	g, ok := s.gates[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return cloneGate(g), nil
}

func (s *Store) UpdateGate(_ context.Context, g *domain.Gate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.gates[g.ID]; !ok {
		return store.ErrNotFound
	}
	s.gates[g.ID] = cloneGate(g)
	s.gateByWI[g.WorkItemID] = g.ID
	return nil
}

func (s *Store) AppendEvent(_ context.Context, e *domain.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[e.WorkItemID] = append(s.events[e.WorkItemID], cloneEvent(e))
	return nil
}

func (s *Store) ListEvents(_ context.Context, workItemID domain.WorkItemID) ([]*domain.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	events := s.events[workItemID]
	out := make([]*domain.Event, 0, len(events))
	for _, e := range events {
		out = append(out, cloneEvent(e))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

func cloneWorkItem(wi *domain.WorkItem) *domain.WorkItem {
	cp := *wi
	if wi.Metadata != nil {
		cp.Metadata = make(domain.Metadata, len(wi.Metadata))
		for k, v := range wi.Metadata {
			cp.Metadata[k] = v
		}
	}
	return &cp
}

func cloneSession(s *domain.Session) *domain.Session {
	cp := *s
	return &cp
}

func cloneGate(g *domain.Gate) *domain.Gate {
	cp := *g
	cp.Approvals = append([]domain.Approval(nil), g.Approvals...)
	cp.Payload.Evidence = append([]domain.EvidenceRef(nil), g.Payload.Evidence...)
	return &cp
}

func cloneEvent(e *domain.Event) *domain.Event {
	cp := *e
	if e.Payload != nil {
		cp.Payload = make(map[string]any, len(e.Payload))
		for k, v := range e.Payload {
			cp.Payload[k] = v
		}
	}
	return &cp
}
