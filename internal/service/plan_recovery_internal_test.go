package service

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stefenello/agent-orchestrator/internal/domain"
)

// Phase 1.2 regression: a legacy session stored only raw stdout. The approved
// plan must still be recoverable from it.
func TestPhaseText_ModernThenLegacyFallback(t *testing.T) {
	legacyRaw := `{"type":"text","timestamp":1,"part":{"type":"text","text":"LEGACY PLAN BODY"}}`
	assert.Equal(t, "LEGACY PLAN BODY", extractAgentText(legacyRaw), "extraction must work on raw stdout")

	legacy := &domain.Session{Output: &domain.SessionOutput{ValidationOutput: legacyRaw}}
	assert.Equal(t, "LEGACY PLAN BODY", phaseText(legacy), "legacy sessions must recover the plan")

	modern := &domain.Session{Output: &domain.SessionOutput{AgentText: "MODERN PLAN", ValidationOutput: legacyRaw}}
	assert.Equal(t, "MODERN PLAN", phaseText(modern), "modern AgentText wins when present")

	assert.Equal(t, "", phaseText(&domain.Session{}), "nil output is safe")
}
