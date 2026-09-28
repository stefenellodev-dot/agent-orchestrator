package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/service"
)

// fakeOpenCodeScript writes a minimal stand-in `opencode` executable and
// returns the directory containing it. The caller prepends it to PATH.
func fakeOpenCodeScript(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "1.18.31"
  exit 0
fi
if [ "$1" = "agent" ] && [ "$2" = "list" ]; then
  echo "build (primary)"
  echo "explore (subagent)"
  exit 0
fi
if [ "$1" = "run" ] && [ "$2" = "--help" ]; then
  printf '%s\n' '--format  format: default or json' '--session' '--continue' '--fork' '--agent' '--dir' '--attach' '--model'
  exit 0
fi
echo "FAKE_RUN_OUTPUT phase=$1"
exit ${FAKE_OPENCODE_EXIT:-0}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "opencode"), []byte(script), 0o755))
	return dir
}

func withFakeOpenCode(t *testing.T) {
	t.Helper()
	dir := fakeOpenCodeScript(t)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestValidateCLI_AgainstFakeBinary(t *testing.T) {
	withFakeOpenCode(t)
	adapter := service.NewCLIAdapter("opencode")

	caps, err := adapter.ValidateCLI(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "1.18.31", caps.Version)
	assert.True(t, caps.SupportsJSON)
	assert.True(t, caps.SupportsDir)
	assert.True(t, caps.SupportsAgent)
	assert.Contains(t, caps.SupportedAgents, "build")
}

func TestRun_Success_CollectsObjectiveEvidence(t *testing.T) {
	withFakeOpenCode(t)
	repo := initRepo(t)
	adapter := service.NewCLIAdapter("opencode")

	res, err := adapter.Run(context.Background(), domain.RunRequest{
		WorktreePath: repo,
		BaseBranch:   "main",
		Phase:        domain.PhaseImplementation,
		Prompt:       "do the thing",
		Agent:        "build",
	})
	require.NoError(t, err)

	assert.Equal(t, 0, res.ExitCode)
	assert.Contains(t, res.Stdout, "FAKE_RUN_OUTPUT")
	assert.NotEmpty(t, res.BaseCommitSHA, "base commit must be captured objectively")
	assert.NotEmpty(t, res.CommitSHA, "head commit must be captured objectively")
}

func TestRun_NonZeroExit_IsEvidenceNotError(t *testing.T) {
	withFakeOpenCode(t)
	repo := initRepo(t)
	adapter := service.NewCLIAdapter("opencode")

	res, err := adapter.Run(context.Background(), domain.RunRequest{
		WorktreePath: repo,
		BaseBranch:   "main",
		Phase:        domain.PhaseValidation,
		Prompt:       "validate",
		EnvVars:      map[string]string{"FAKE_OPENCODE_EXIT": "3"},
	})
	require.NoError(t, err, "a failing command is still a valid result")
	assert.Equal(t, 3, res.ExitCode)
}

func TestRun_IncludesOnlyVerifiedFlags(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then echo "1.18.31"; exit 0; fi
if [ "$1" = "agent" ] && [ "$2" = "list" ]; then echo "build (primary)"; exit 0; fi
if [ "$1" = "run" ] && [ "$2" = "--help" ]; then
  printf '%s\n' '--format default or json' '--session' '--continue' '--fork' '--agent' '--dir' '--attach' '--model'
  exit 0
fi
echo "ARGS: $@"
exit 0
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "opencode"), []byte(script), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	repo := initRepo(t)
	adapter := service.NewCLIAdapter("opencode")
	res, err := adapter.Run(context.Background(), domain.RunRequest{
		WorktreePath: repo,
		Phase:        domain.PhaseDecision,
		Prompt:       "plan it",
		Agent:        "plan",
		Model:        "opencode-go/deepseek-v4-flash",
	})
	require.NoError(t, err)
	assert.Contains(t, res.Stdout, "--agent plan")
	assert.Contains(t, res.Stdout, "--model opencode-go/deepseek-v4-flash")
	assert.Contains(t, res.Stdout, "--dir "+repo)
}

func TestRun_MalformedOutputDoesNotPanic(t *testing.T) {
	withFakeOpenCode(t)
	repo := initRepo(t)
	adapter := service.NewCLIAdapter("opencode")

	assert.NotPanics(t, func() {
		res, err := adapter.Run(context.Background(), domain.RunRequest{
			WorktreePath: repo,
			BaseBranch:   "main",
			Phase:        domain.PhaseDiscovery,
			Prompt:       "explore",
		})
		require.NoError(t, err)
		assert.Equal(t, 0, res.ExitCode)
	})
}
