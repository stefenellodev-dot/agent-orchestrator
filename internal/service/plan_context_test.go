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

func TestRun_ExtractsAgentTextFromJSONStream(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then echo "1.18.31"; exit 0; fi
if [ "$1" = "agent" ] && [ "$2" = "list" ]; then echo "build (primary)"; exit 0; fi
if [ "$1" = "run" ] && [ "$2" = "--help" ]; then
  printf '%s\n' '--format default or json' '--agent' '--dir' '--model'
  exit 0
fi
echo '{"type":"text","part":{"type":"text","text":"HELLO PLAN"}}'
echo '{"type":"text","part":{"type":"text","text":"SECOND PARAGRAPH"}}'
echo '{"type":"step_finish","part":{}}'
exit 0
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "opencode"), []byte(script), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	res, err := service.NewCLIAdapter("opencode").Run(context.Background(), domain.RunRequest{
		WorktreePath: initRepo(t),
		Phase:        domain.PhaseDiscovery,
		Prompt:       "x",
	})
	require.NoError(t, err)
	assert.Equal(t, "HELLO PLAN\nSECOND PARAGRAPH", res.AgentText)
}

func TestPromptFor_ImplementationCarriesApprovedPlan(t *testing.T) {
	wi := &domain.WorkItem{ID: "WI-1", Title: "T", Description: "D", BaseBranch: "main"}

	impl := service.PromptFor(wi, domain.PhaseImplementation, "PLAN-BODY-XYZ")
	assert.Contains(t, impl, "APPROVED PLAN")
	assert.Contains(t, impl, "PLAN-BODY-XYZ")

	assert.NotContains(t, service.PromptFor(wi, domain.PhaseDiscovery, "PLAN-BODY-XYZ"), "PLAN-BODY-XYZ",
		"discovery must not receive the plan")
}
