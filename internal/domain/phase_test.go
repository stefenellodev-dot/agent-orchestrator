package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPhaseNext(t *testing.T) {
	assert.Equal(t, PhaseDecision, PhaseDiscovery.Next())
	assert.Equal(t, PhaseAwaitingApproval, PhaseDecision.Next())
	assert.Equal(t, PhaseImplementation, PhaseAwaitingApproval.Next())
	assert.Equal(t, PhaseValidation, PhaseImplementation.Next())
	assert.Equal(t, PhaseComplete, PhaseValidation.Next())
	assert.Equal(t, PhaseComplete, PhaseComplete.Next(), "terminal phases are their own next")
}

func TestPhaseIsTerminal(t *testing.T) {
	assert.True(t, PhaseComplete.IsTerminal())
	assert.True(t, PhaseFailed.IsTerminal())
	assert.True(t, PhaseBlocked.IsTerminal())
	assert.False(t, PhaseDiscovery.IsTerminal())
	assert.False(t, PhaseAwaitingApproval.IsTerminal())
}

func TestPhaseRequiresOpenCodeRun(t *testing.T) {
	assert.True(t, PhaseDiscovery.RequiresOpenCodeRun())
	assert.True(t, PhaseDecision.RequiresOpenCodeRun())
	assert.True(t, PhaseImplementation.RequiresOpenCodeRun())
	assert.True(t, PhaseValidation.RequiresOpenCodeRun())
	assert.False(t, PhaseAwaitingApproval.RequiresOpenCodeRun())
	assert.False(t, PhaseComplete.RequiresOpenCodeRun())
}

func TestNewIDsAreUniqueAndOrdered(t *testing.T) {
	a := NewWorkItemID()
	b := NewWorkItemID()
	assert.NotEqual(t, a, b)
	assert.NotEmpty(t, string(a))
}
