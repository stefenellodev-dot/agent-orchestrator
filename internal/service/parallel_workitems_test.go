package service_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/service"
	"github.com/stefenello/agent-orchestrator/internal/store/memory"
)

// probeAdapter records maximum observed concurrency and can fail to slow/select
// specific WorkItems via a marker in the prompt.
type probeAdapter struct {
	mu          sync.Mutex
	inflight    int
	maxInflight int
	runDelay    time.Duration
	slowMarker  string
	failMarker  string
}

func (a *probeAdapter) Name() string { return "probe" }
func (a *probeAdapter) Available(context.Context) (domain.CLICapabilities, error) {
	return domain.CLICapabilities{Version: "probe"}, nil
}
func (a *probeAdapter) Run(_ context.Context, req domain.RunRequest) (*domain.RunResult, error) {
	a.mu.Lock()
	a.inflight++
	if a.inflight > a.maxInflight {
		a.maxInflight = a.inflight
	}
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.inflight--
		a.mu.Unlock()
	}()

	switch {
	case a.failMarker != "" && strings.Contains(req.Prompt, a.failMarker):
		return &domain.RunResult{ExitCode: 1, Stderr: "boom"}, nil
	case a.slowMarker != "" && strings.Contains(req.Prompt, a.slowMarker):
		return &domain.RunResult{TimedOut: true, ExitCode: -1}, nil
	case a.runDelay > 0:
		time.Sleep(a.runDelay)
	}
	return &domain.RunResult{ExitCode: 0}, nil
}
func (a *probeAdapter) max() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.maxInflight
}

func newParallelOrch(t *testing.T, ad domain.Runtime, limit int) (*service.Orchestrator, *memory.Store) {
	t.Helper()
	st := memory.New()
	orch := service.New(st, &fakeProvisioner{}, ad)
	orch.SetMaxActivePerProject(limit)
	return orch, st
}

func TestParallel_TwoConcurrentWorkItems(t *testing.T) {
	ctx := context.Background()
	ad := &probeAdapter{runDelay: 200 * time.Millisecond}
	orch, _ := newParallelOrch(t, ad, 2)

	a, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "A", RepoPath: "/repo"})
	require.NoError(t, err)
	b, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "B", RepoPath: "/repo"})
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = orch.Drive(ctx, a.ID) }()
	go func() { defer wg.Done(); _ = orch.Drive(ctx, b.ID) }()
	wg.Wait()

	assert.GreaterOrEqual(t, ad.max(), 2, "two WorkItems must execute concurrently")

	// Independent worktrees.
	ga, _ := orch.GetWorkItem(ctx, a.ID)
	gb, _ := orch.GetWorkItem(ctx, b.ID)
	assert.NotEqual(t, ga.WorktreePath, gb.WorktreePath, "worktrees must be independent")

	// Independent sessions/evidence (each session belongs to its WorkItem).
	sa, _ := orch.ListSessions(ctx, a.ID)
	sb, _ := orch.ListSessions(ctx, b.ID)
	require.NotEmpty(t, sa)
	require.NotEmpty(t, sb)
	for _, s := range sa {
		assert.Equal(t, a.ID, s.WorkItemID)
	}
	for _, s := range sb {
		assert.Equal(t, b.ID, s.WorkItemID)
	}
}

func TestParallel_ConcurrencyLimitRejects(t *testing.T) {
	ctx := context.Background()
	orch, _ := newParallelOrch(t, &probeAdapter{}, 2)

	_, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "A", RepoPath: "/repo"})
	require.NoError(t, err)
	_, err = orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "B", RepoPath: "/repo"})
	require.NoError(t, err)
	_, err = orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "C", RepoPath: "/repo"})
	assert.ErrorIs(t, err, service.ErrProjectBusy, "third WorkItem must wait / be rejected")
}

func TestParallel_FailureIsolation(t *testing.T) {
	ctx := context.Background()
	ad := &probeAdapter{failMarker: "FAILA"}
	orch, _ := newParallelOrch(t, ad, 2)

	a, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "FAILA work", RepoPath: "/repo"})
	require.NoError(t, err)
	b, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "B work", RepoPath: "/repo"})
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = orch.Drive(ctx, a.ID) }()
	go func() { defer wg.Done(); _ = orch.Drive(ctx, b.ID) }()
	wg.Wait()

	ga, _ := orch.GetWorkItem(ctx, a.ID)
	gb, _ := orch.GetWorkItem(ctx, b.ID)
	assert.Equal(t, domain.PhaseFailed, ga.CurrentPhase, "A must fail")
	assert.Equal(t, domain.PhaseAwaitingApproval, gb.CurrentPhase, "B must be unaffected by A's failure")
}

func TestParallel_TimeoutIsolation(t *testing.T) {
	ctx := context.Background()
	ad := &probeAdapter{slowMarker: "SLOWA"}
	orch, _ := newParallelOrch(t, ad, 2)

	a, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "SLOWA work", RepoPath: "/repo"})
	require.NoError(t, err)
	b, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "B work", RepoPath: "/repo"})
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = orch.Drive(ctx, a.ID) }()
	go func() { defer wg.Done(); _ = orch.Drive(ctx, b.ID) }()
	wg.Wait()

	ga, _ := orch.GetWorkItem(ctx, a.ID)
	gb, _ := orch.GetWorkItem(ctx, b.ID)
	assert.Equal(t, domain.PhaseFailed, ga.CurrentPhase, "A (timeout) must fail")

	sa, _ := orch.ListSessions(ctx, a.ID)
	require.NotEmpty(t, sa)
	assert.Equal(t, "runtime timeout", sa[0].Error)

	assert.Equal(t, domain.PhaseAwaitingApproval, gb.CurrentPhase, "B must be unaffected by A's timeout")
}

func TestParallel_RecoveryMultipleWorkItems(t *testing.T) {
	ctx := context.Background()
	orch, st := newParallelOrch(t, &probeAdapter{}, 2)

	a, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "A", RepoPath: "/repo"})
	require.NoError(t, err)
	b, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "B", RepoPath: "/repo"})
	require.NoError(t, err)

	// Simulate a crash: both have an interrupted (running) session.
	sa := addRunningSession(t, st, a, domain.PhaseDiscovery)
	sb := addRunningSession(t, st, b, domain.PhaseDiscovery)

	n, err := orch.Reconcile(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, n, "both interrupted WorkItems must be reconciled")

	// Idempotent: second pass touches nothing new.
	_, err = orch.Reconcile(ctx)
	require.NoError(t, err)

	// The interrupted sessions are marked failed (evidence preserved).
	gotA, _ := st.GetSession(ctx, sa)
	gotB, _ := st.GetSession(ctx, sb)
	assert.Equal(t, domain.SessionFailed, gotA.Status)
	assert.Equal(t, domain.SessionFailed, gotB.Status)
}

func TestParallel_DiagnosticsMultipleWorkItems(t *testing.T) {
	ctx := context.Background()
	orch, st := newParallelOrch(t, &probeAdapter{}, 2)

	a, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "A", RepoPath: "/repo"})
	require.NoError(t, err)
	b, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "B", RepoPath: "/repo"})
	require.NoError(t, err)
	addRunningSession(t, st, a, domain.PhaseDiscovery)
	addRunningSession(t, st, b, domain.PhaseDiscovery)

	rep := orch.Diagnostics(ctx)
	assert.False(t, hasFinding(rep, "workitems", service.StatusWarn), "consistent parallel WorkItems must not warn")
	assert.False(t, hasFinding(rep, "sessions", service.StatusWarn))
}
