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

type fakeProvisioner struct {
	createCalls int
	cleaned     bool
}

func (f *fakeProvisioner) Create(_ context.Context, workItemID, _, _ string) (*service.Worktree, error) {
	f.createCalls++
	return &service.Worktree{WorkItemID: workItemID, Path: "/tmp/wt/" + workItemID}, nil
}

func (f *fakeProvisioner) Cleanup(_ context.Context, _ string) error {
	f.cleaned = true
	return nil
}

func (f *fakeProvisioner) Path(workItemID string) string { return "/tmp/wt/" + workItemID }
