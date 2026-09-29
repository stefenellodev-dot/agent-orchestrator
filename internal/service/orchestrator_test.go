package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/service"
	"github.com/stefenello/agent-orchestrator/internal/store/memory"
)

func eventTypes(events []*domain.Event) []domain.EventType {
	out := make([]domain.EventType, 0, len(events))
	for _, e := range events {
		out = append(out, e.Type)
	}
	return out
}

func TestCreateWorkItem_StartsDiscoveryAndEmitsEvents(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	provisioner := &fakeProvisioner{}
	svc := service.New(store, provisioner, nil)

	wi, err := svc.CreateWorkItem(ctx, service.CreateWorkItemInput{
		Project:     "test-project",
		Title:       "Add widget",
		Description: "A widget is needed",
		BaseBranch:  "main",
		RepoPath:    "/tmp/repo",
	})
	require.NoError(t, err)

	assert.NotEmpty(t, wi.ID)
	assert.Equal(t, domain.PhaseDiscovery, wi.CurrentPhase)
	assert.Equal(t, domain.PhaseDiscovery, wi.Status)
	assert.NotEmpty(t, wi.WorktreePath)
	assert.Equal(t, 1, provisioner.createCalls)

	stored, err := store.GetWorkItem(ctx, wi.ID)
	require.NoError(t, err)
	assert.Equal(t, wi.ID, stored.ID)

	events, err := store.ListEvents(ctx, wi.ID)
	require.NoError(t, err)
	types := eventTypes(events)
	assert.Contains(t, types, domain.EventWorkItemCreated)
	assert.Contains(t, types, domain.EventPhaseStarted)
}

func TestCreateWorkItem_RequiresProjectAndTitle(t *testing.T) {
	ctx := context.Background()
	svc := service.New(memory.New(), &fakeProvisioner{}, nil)

	_, err := svc.CreateWorkItem(ctx, service.CreateWorkItemInput{Title: "no project"})
	assert.ErrorIs(t, err, service.ErrInvalidInput)

	_, err = svc.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p"})
	assert.ErrorIs(t, err, service.ErrInvalidInput)
}

func TestGetWorkItem_NotFound(t *testing.T) {
	ctx := context.Background()
	svc := service.New(memory.New(), &fakeProvisioner{}, nil)

	_, err := svc.GetWorkItem(ctx, "does-not-exist")
	assert.ErrorIs(t, err, service.ErrNotFound)
}

func TestCreateWorkItem_ResolvesRepoPathFromProjectConfig(t *testing.T) {
	ctx := context.Background()
	prov := &fakeProvisioner{}
	svc := service.New(memory.New(), prov, nil)
	svc.RegisterProject(service.ProjectConfig{
		Name:       "condosmart",
		RepoPath:   "/srv/condosmart",
		BaseBranch: "master",
	})

	wi, err := svc.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "condosmart", Title: "t"})
	require.NoError(t, err)
	assert.Equal(t, "master", wi.BaseBranch)
	assert.Equal(t, "/srv/condosmart", prov.lastRepoPath)
	assert.Equal(t, "master", prov.lastBase)
}

// Phase 3/4 regression: every registered project resolves its own repository
// and base branch without special-casing the orchestrator's own repository.
func TestRegisterProject_AllProjectsResolveIndependently(t *testing.T) {
	ctx := context.Background()
	svc := service.New(memory.New(), &fakeProvisioner{}, nil)
	projects := []service.ProjectConfig{
		{Name: "condosmart", RepoPath: "/repos/movil", BaseBranch: "master"},
		{Name: "agent-orchestrator", RepoPath: "/repos/agent-orchestrator", BaseBranch: "main"},
		{Name: "fin-engine-v1", RepoPath: "/repos/fin-engine-desa", BaseBranch: "qa"},
	}
	for _, pc := range projects {
		svc.RegisterProject(pc)
	}

	listed := svc.ProjectList()
	require.Len(t, listed, 3)
	assert.Equal(t, "agent-orchestrator", listed[0].Name) // sorted

	for _, pc := range projects {
		prov := &fakeProvisioner{}
		svcWithProv := service.New(memory.New(), prov, nil)
		for _, p := range projects {
			svcWithProv.RegisterProject(p)
		}
		wi, err := svcWithProv.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: pc.Name, Title: "t"})
		require.NoError(t, err)
		assert.Equal(t, pc.BaseBranch, wi.BaseBranch)
		assert.Equal(t, pc.RepoPath, prov.lastRepoPath)
	}
}

type fakeProvisioner struct {
	createCalls  int
	cleanupCalls int
	cleaned      bool
	lastRepoPath string
	lastBase     string
}

func (f *fakeProvisioner) Create(_ context.Context, workItemID, repoPath, baseBranch string) (*service.Worktree, error) {
	f.createCalls++
	f.lastRepoPath = repoPath
	f.lastBase = baseBranch
	return &service.Worktree{WorkItemID: workItemID, Path: "/tmp/wt/" + workItemID}, nil
}

func (f *fakeProvisioner) Cleanup(_ context.Context, _ string) error {
	f.cleanupCalls++
	f.cleaned = true
	return nil
}

func (f *fakeProvisioner) Path(workItemID string) string { return "/tmp/wt/" + workItemID }

func (f *fakeProvisioner) List() ([]string, error) { return nil, nil }
