package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefenello/agent-orchestrator/internal/service"
	"github.com/stefenello/agent-orchestrator/internal/store/memory"
)

func TestCreateWorkItem_SecondActiveForSameProjectIsBusy(t *testing.T) {
	ctx := context.Background()
	svc := service.New(memory.New(), &fakeProvisioner{}, nil)

	_, err := svc.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "first", RepoPath: "/tmp/repo"})
	require.NoError(t, err)

	_, err = svc.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "second", RepoPath: "/tmp/repo"})
	assert.ErrorIs(t, err, service.ErrProjectBusy)

	// A different project is unaffected.
	_, err = svc.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "q", Title: "other", RepoPath: "/tmp/repo"})
	require.NoError(t, err)
}

func TestCreateWorkItem_BusyDoesNotLeakWorktree(t *testing.T) {
	ctx := context.Background()
	provisioner := &fakeProvisioner{}
	svc := service.New(memory.New(), provisioner, nil)

	_, err := svc.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "first", RepoPath: "/tmp/repo"})
	require.NoError(t, err)
	created := provisioner.createCalls

	_, err = svc.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "second", RepoPath: "/tmp/repo"})
	require.ErrorIs(t, err, service.ErrProjectBusy)

	// The rejected attempt must not leave an orphaned worktree behind.
	assert.Equal(t, created+1, provisioner.createCalls, "worktree was provisioned before the busy check")
	assert.Equal(t, 1, provisioner.cleanupCalls, "orphaned worktree must be cleaned up")
}
