package service_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/service"
	"github.com/stefenello/agent-orchestrator/internal/store/memory"
)

const probeBody = `if [ "$1" = "--version" ]; then echo "1.18.31"; exit 0; fi
if [ "$1" = "agent" ] && [ "$2" = "list" ]; then echo "build (primary)"; exit 0; fi
if [ "$1" = "run" ] && [ "$2" = "--help" ]; then printf '%s\n' '--format default or json' '--agent' '--dir' '--model'; exit 0; fi
`

func writeRuntime(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "opencode")
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755))
	return p
}

func TestRuntime_AvailableHealthy(t *testing.T) {
	ctx := context.Background()
	a := service.NewCLIAdapter(writeRuntime(t, probeBody+"exit 0\n"))
	caps, err := a.Available(ctx)
	require.NoError(t, err)
	assert.Equal(t, "1.18.31", caps.Version)
	assert.True(t, caps.SupportsAgent)
	assert.Equal(t, "opencode", a.Name())

	// Idempotent.
	caps2, err := a.Available(ctx)
	require.NoError(t, err)
	assert.Equal(t, caps.Version, caps2.Version)
}

func TestRuntime_AvailableUnavailable(t *testing.T) {
	a := service.NewCLIAdapter("/nonexistent/opencode-does-not-exist")
	_, err := a.Available(context.Background())
	assert.ErrorIs(t, err, domain.ErrRuntimeUnavailable)
}

func TestRuntime_ProcessStartFailure(t *testing.T) {
	ctx := context.Background()
	bin := writeRuntime(t, probeBody+"exit 0\n")
	a := service.NewCLIAdapter(bin)
	_, err := a.Available(ctx)
	require.NoError(t, err) // probed OK

	require.NoError(t, os.Chmod(bin, 0o644)) // no longer executable
	_, err = a.Run(ctx, domain.RunRequest{WorktreePath: t.TempDir(), Prompt: "x"})
	assert.ErrorIs(t, err, domain.ErrProcessStart)
}

func TestRuntime_NonZeroExitAndStreams(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	bin := writeRuntime(t, probeBody+"echo OUT_LINE; echo ERR_LINE 1>&2; exit 3\n")
	a := service.NewCLIAdapter(bin)
	_, err := a.Available(ctx)
	require.NoError(t, err)

	res, err := a.Run(ctx, domain.RunRequest{WorktreePath: repo, BaseBranch: "main", Prompt: "x"})
	require.NoError(t, err, "a non-zero exit is evidence, not an error")
	assert.Equal(t, 3, res.ExitCode)
	assert.Contains(t, res.Stdout, "OUT_LINE")
	assert.Contains(t, res.Stderr, "ERR_LINE")
	assert.False(t, res.TimedOut)
	assert.False(t, res.Canceled)
	assert.NotEmpty(t, res.CommitSHA, "evidence preserved on failure")
}

func TestRuntime_Timeout(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	bin := writeRuntime(t, probeBody+"sleep 30\n")
	a := service.NewCLIAdapter(bin)
	_, err := a.Available(ctx)
	require.NoError(t, err)

	start := time.Now()
	res, err := a.Run(ctx, domain.RunRequest{WorktreePath: repo, Prompt: "x", Timeout: 400 * time.Millisecond})
	require.NoError(t, err)
	assert.True(t, res.TimedOut)
	assert.False(t, res.Canceled)
	assert.Less(t, time.Since(start), 10*time.Second, "must not hang past the timeout")
}

func TestRuntime_Cancellation(t *testing.T) {
	repo := initRepo(t)
	bin := writeRuntime(t, probeBody+"sleep 30\n")
	a := service.NewCLIAdapter(bin)
	_, err := a.Available(context.Background())
	require.NoError(t, err)

	cctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(300 * time.Millisecond); cancel() }()
	res, err := a.Run(cctx, domain.RunRequest{WorktreePath: repo, Prompt: "x", Timeout: 30 * time.Second})
	require.NoError(t, err)
	assert.True(t, res.Canceled)
	assert.False(t, res.TimedOut)
}

func TestRuntime_NoOrphanProcessOnTimeout(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	pidfile := filepath.Join(t.TempDir(), "pid")
	bin := writeRuntime(t, probeBody+`echo $$ > "$PIDFILE"; sleep 30`+"\n")
	a := service.NewCLIAdapter(bin)
	_, err := a.Available(ctx)
	require.NoError(t, err)

	res, err := a.Run(ctx, domain.RunRequest{
		WorktreePath: repo, Prompt: "x", Timeout: 500 * time.Millisecond,
		EnvVars: map[string]string{"PIDFILE": pidfile},
	})
	require.NoError(t, err)
	require.True(t, res.TimedOut)

	data, err := os.ReadFile(pidfile)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return syscall.Kill(pid, 0) != nil // ESRCH → process gone
	}, 5*time.Second, 50*time.Millisecond, "child process must be killed (no orphan)")
}

func TestRuntime_DiagnosticsIntegration(t *testing.T) {
	ctx := context.Background()
	// A runtime pointing at a missing binary → diagnostics reports worker failure.
	orch := service.New(memory.New(), &fakeProvisioner{}, service.NewCLIAdapter("/nonexistent/opencode"))
	rep := orch.Diagnostics(ctx)
	assert.True(t, hasFinding(rep, "worker", service.StatusFail))
	assert.Equal(t, service.OverallUnhealthy, rep.Overall)
}
