package service_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/service"
	"github.com/stefenello/agent-orchestrator/internal/store/memory"
)

// failingStore simulates a database outage for a safe read.
type failingStore struct{ *memory.Store }

func (f failingStore) ListWorkItems(context.Context, string) ([]*domain.WorkItem, error) {
	return nil, errors.New("db down")
}

// failingAdapter simulates an unavailable OpenCode worker.
type failingAdapter struct{}

func (failingAdapter) Name() string { return "opencode" }

func (failingAdapter) Available(context.Context) (domain.CLICapabilities, error) {
	return domain.CLICapabilities{}, errors.New("worker down")
}
func (failingAdapter) Run(context.Context, domain.RunRequest) (*domain.RunResult, error) {
	return nil, errors.New("worker down")
}

func findingsFor(rep service.DiagnosticsReport, check string) []service.Finding {
	var out []service.Finding
	for _, f := range rep.Findings {
		if f.Check == check {
			out = append(out, f)
		}
	}
	return out
}

func hasFinding(rep service.DiagnosticsReport, check, status string) bool {
	for _, f := range rep.Findings {
		if f.Check == check && f.Status == status {
			return true
		}
	}
	return false
}

func TestDiagnostics_HealthySystem(t *testing.T) {
	ctx := context.Background()
	orch, st, repo, _ := newGCOrch(t)
	id := createWI(t, orch, repo, "p")
	wi, _ := orch.GetWorkItem(ctx, id)
	addRunningSession(t, st, wi, domain.PhaseDiscovery) // consistent in-flight execution

	rep := orch.Diagnostics(ctx)
	assert.Equal(t, service.OverallOK, rep.Overall)
	assert.True(t, hasFinding(rep, "database", service.StatusOK))
	assert.True(t, hasFinding(rep, "worker", service.StatusOK))
	assert.Empty(t, findingsFor(rep, "worktree"))
	assert.False(t, hasFinding(rep, "workitems", service.StatusWarn))
	assert.False(t, hasFinding(rep, "sessions", service.StatusWarn))
}

func TestDiagnostics_DatabaseUnavailable(t *testing.T) {
	orch := service.New(failingStore{memory.New()}, &fakeProvisioner{}, &fakeAdapter{})
	rep := orch.Diagnostics(context.Background())
	assert.Equal(t, service.OverallUnhealthy, rep.Overall)
	assert.True(t, hasFinding(rep, "database", service.StatusFail))
}

func TestDiagnostics_WorkerUnavailable(t *testing.T) {
	orch := service.New(memory.New(), &fakeProvisioner{}, failingAdapter{})
	rep := orch.Diagnostics(context.Background())
	assert.Equal(t, service.OverallUnhealthy, rep.Overall)
	assert.True(t, hasFinding(rep, "worker", service.StatusFail))
}

func TestDiagnostics_NonTerminalWithoutExecutionEvidence(t *testing.T) {
	ctx := context.Background()
	orch, _, repo, _ := newGCOrch(t)
	createWI(t, orch, repo, "p") // discovery, no sessions at all
	rep := orch.Diagnostics(ctx)
	assert.True(t, hasFinding(rep, "workitems", service.StatusWarn))
	assert.Equal(t, service.OverallDegraded, rep.Overall)
}

func TestDiagnostics_TerminalWorkItemWithActiveSession(t *testing.T) {
	ctx := context.Background()
	orch, st, repo, _ := newGCOrch(t)
	id := createWI(t, orch, repo, "p")
	wi, _ := orch.GetWorkItem(ctx, id)
	addRunningSession(t, st, wi, domain.PhaseDiscovery)
	setPhase(t, st, id, domain.PhaseComplete) // terminal but a session is still active

	rep := orch.Diagnostics(ctx)
	assert.True(t, hasFinding(rep, "sessions", service.StatusWarn))
}

func TestDiagnostics_WorktreeUnknownDetectedNotDeleted(t *testing.T) {
	ctx := context.Background()
	orch, _, repo, root := newGCOrch(t)
	fakeID := "01DIAGUNKNOWN0000000000000"
	path := filepath.Join(root, fakeID)
	runGit(t, repo, "worktree", "add", "-b", "workitem/"+fakeID, path, "main")

	rep := orch.Diagnostics(ctx)
	assert.True(t, hasFinding(rep, "worktree", service.StatusWarn))
	assert.DirExists(t, path, "diagnostics must never delete an unknown worktree")
}

func TestDiagnostics_ExpectedWorktreeMissing(t *testing.T) {
	ctx := context.Background()
	orch, st, repo, _ := newGCOrch(t)
	id := createWI(t, orch, repo, "p")
	wi, _ := orch.GetWorkItem(ctx, id)
	addRunningSession(t, st, wi, domain.PhaseDiscovery)
	require.NoError(t, os.RemoveAll(wi.WorktreePath))

	rep := orch.Diagnostics(ctx)
	assert.True(t, hasFinding(rep, "worktree", service.StatusWarn))
}

func TestDiagnostics_DoesNotMutateState(t *testing.T) {
	ctx := context.Background()
	orch, st, repo, _ := newGCOrch(t)
	id := createWI(t, orch, repo, "p")
	wi, _ := orch.GetWorkItem(ctx, id)
	addRunningSession(t, st, wi, domain.PhaseDiscovery)

	beforePhase := wi.CurrentPhase
	beforeEvents, _ := orch.ListEvents(ctx, id)
	beforeSessions, _ := orch.ListSessions(ctx, id)
	require.DirExists(t, wi.WorktreePath)

	rep1 := orch.Diagnostics(ctx)
	rep2 := orch.Diagnostics(ctx)

	after, _ := orch.GetWorkItem(ctx, id)
	assert.Equal(t, beforePhase, after.CurrentPhase, "diagnostics must not change WorkItem state")
	afterEvents, _ := orch.ListEvents(ctx, id)
	afterSessions, _ := orch.ListSessions(ctx, id)
	assert.Len(t, afterEvents, len(beforeEvents), "diagnostics must not create events")
	assert.Len(t, afterSessions, len(beforeSessions), "diagnostics must not create sessions")
	assert.DirExists(t, wi.WorktreePath, "diagnostics must not touch worktrees")

	// Deterministic / idempotent across repeated runs.
	assert.Equal(t, rep1.Overall, rep2.Overall)
	assert.Len(t, rep1.Findings, len(rep2.Findings))
}
