package service_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/service"
)

// Recorded output of `opencode run --help` (v1.18.31). Kept verbatim so the
// probe is validated against the real, verified CLI surface.
const realRunHelp = `opencode run [message..]

run opencode with a message

Options:
  -c, --continue     continue the last session                                             [boolean]
  -s, --session      session id to continue                                                 [string]
      --fork         fork the session before continuing (requires --continue or --session) [boolean]
  -m, --model        model to use in the format of provider/model                           [string]
      --agent        agent to use                                                           [string]
      --format       format: default (formatted) or json (raw JSON events)
                                          [string] [choices: "default", "json"] [default: "default"]
      --attach       attach to a running opencode server (e.g., http://localhost:4096)      [string]
      --dir          directory to run in, path on remote server if attaching                [string]
      --auto         auto-approve permissions that are not explicitly denied (dangerous!)
`

func TestParseCapabilities_RealHelpText(t *testing.T) {
	caps := service.ParseCapabilities("1.18.31", realRunHelp, "build (primary)\nexplore (subagent)\n")

	assert.Equal(t, "1.18.31", caps.Version)
	assert.True(t, caps.SupportsJSON)
	assert.True(t, caps.SupportsSession)
	assert.True(t, caps.SupportsContinue)
	assert.True(t, caps.SupportsFork)
	assert.True(t, caps.SupportsAgent)
	assert.True(t, caps.SupportsDir)
	assert.True(t, caps.SupportsAttach)
	assert.True(t, caps.SupportsModel)
	assert.Contains(t, caps.SupportedAgents, "build")
	assert.Contains(t, caps.SupportedAgents, "explore")
}

func TestParseCapabilities_EmptyInputDoesNotPanic(t *testing.T) {
	var caps domain.CLICapabilities
	assert.NotPanics(t, func() { caps = service.ParseCapabilities("", "", "") })

	assert.False(t, caps.SupportsJSON)
	assert.False(t, caps.SupportsSession)
	assert.False(t, caps.SupportsFork)
	assert.False(t, caps.SupportsAgent)
	assert.False(t, caps.SupportsDir)
	assert.Empty(t, caps.SupportedAgents)
}

func TestParseCapabilities_UnknownHelpMarksAbsent(t *testing.T) {
	caps := service.ParseCapabilities("9.9.9", "totally different command\n", "")
	assert.False(t, caps.SupportsJSON)
	assert.False(t, caps.SupportsSession)
	assert.False(t, caps.SupportsDir)
	assert.Equal(t, "9.9.9", caps.Version)
}
