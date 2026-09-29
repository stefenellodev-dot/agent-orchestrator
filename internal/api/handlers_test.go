package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefenello/agent-orchestrator/internal/api"
	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/service"
	"github.com/stefenello/agent-orchestrator/internal/store"
)

type stubService struct {
	items        map[domain.WorkItemID]*domain.WorkItem
	events       map[domain.WorkItemID][]*domain.Event
	gates        map[domain.WorkItemID]*domain.Gate
	lastApproval service.ApprovalInput
	lastRetry    service.RetryInput
}

func newStubService() *stubService {
	return &stubService{
		items:  map[domain.WorkItemID]*domain.WorkItem{},
		events: map[domain.WorkItemID][]*domain.Event{},
		gates:  map[domain.WorkItemID]*domain.Gate{},
	}
}

func (s *stubService) GetGate(_ context.Context, id domain.WorkItemID) (*domain.Gate, error) {
	g, ok := s.gates[id]
	if !ok {
		return nil, service.ErrNotFound
	}
	return g, nil
}

func (s *stubService) Approve(_ context.Context, _ domain.WorkItemID, in service.ApprovalInput) error {
	s.lastApproval = in
	return nil
}

func (s *stubService) Reject(_ context.Context, _ domain.WorkItemID, in service.ApprovalInput) error {
	s.lastApproval = in
	return nil
}

func (s *stubService) RequestChanges(_ context.Context, _ domain.WorkItemID, in service.ApprovalInput) error {
	s.lastApproval = in
	return nil
}

func (s *stubService) Retry(_ context.Context, _ domain.WorkItemID, in service.RetryInput) error {
	s.lastRetry = in
	return nil
}

func (s *stubService) ProjectList() []service.ProjectConfig {
	return []service.ProjectConfig{{Name: "p", RepoPath: "/repo", BaseBranch: "main"}}
}

func (s *stubService) Diagnostics(_ context.Context) service.DiagnosticsReport {
	return service.DiagnosticsReport{Overall: service.OverallOK}
}

func (s *stubService) CreateWorkItem(_ context.Context, in service.CreateWorkItemInput) (*domain.WorkItem, error) {
	if in.Project == "" || in.Title == "" {
		return nil, service.ErrInvalidInput
	}
	wi := &domain.WorkItem{
		ID:           domain.NewWorkItemID(),
		Project:      in.Project,
		Title:        in.Title,
		Description:  in.Description,
		Priority:     domain.PriorityMedium,
		Status:       domain.PhaseDiscovery,
		CurrentPhase: domain.PhaseDiscovery,
		WorktreePath: "/tmp/wt/" + string(domain.NewWorkItemID()),
		BaseBranch:   "main",
	}
	s.items[wi.ID] = wi
	s.events[wi.ID] = []*domain.Event{{ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkItemCreated, Actor: domain.ActorHuman}}
	return wi, nil
}

func (s *stubService) GetWorkItem(_ context.Context, id domain.WorkItemID) (*domain.WorkItem, error) {
	wi, ok := s.items[id]
	if !ok {
		return nil, service.ErrNotFound
	}
	return wi, nil
}

func (s *stubService) ListWorkItems(_ context.Context, project string) ([]*domain.WorkItem, error) {
	var out []*domain.WorkItem
	for _, wi := range s.items {
		if project == "" || wi.Project == project {
			out = append(out, wi)
		}
	}
	return out, nil
}

func (s *stubService) ListEvents(_ context.Context, id domain.WorkItemID) ([]*domain.Event, error) {
	if _, ok := s.items[id]; !ok {
		return nil, service.ErrNotFound
	}
	return s.events[id], nil
}

func (s *stubService) ListSessions(_ context.Context, id domain.WorkItemID) ([]*domain.Session, error) {
	if _, ok := s.items[id]; !ok {
		return nil, service.ErrNotFound
	}
	return []*domain.Session{}, nil
}

var _ api.OrchestratorService = (*stubService)(nil)
var _ = store.ErrNotFound

func newTestServer() http.Handler {
	return api.NewServer(newStubService(), api.Options{})
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	newTestServer().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCreateAndGetWorkItem(t *testing.T) {
	e := newTestServer()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/workitems",
		strings.NewReader(`{"project":"p","title":"t","description":"d"}`))
	req.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var created domain.WorkItem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, domain.PhaseDiscovery, created.CurrentPhase)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/workitems/"+string(created.ID), nil)
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var got domain.WorkItem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, created.ID, got.ID)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/workitems/"+string(created.ID)+"/events", nil)
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var events []domain.Event
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &events))
	require.Len(t, events, 1)
	assert.Equal(t, domain.EventWorkItemCreated, events[0].Type)
}

func TestCreateWorkItem_ValidationError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/workitems", strings.NewReader(`{"title":"no project"}`))
	req.Header.Set("Content-Type", "application/json")
	newTestServer().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGetWorkItem_NotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/workitems/nope", nil)
	newTestServer().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
