package service

import (
	"fmt"
	"strings"

	"github.com/stefenello/agent-orchestrator/internal/domain"
)

// PromptFor renders the phase prompt for a WorkItem. Phases are independent:
// correctness must not depend on OpenCode session continuity. Continuity comes
// from the shared worktree on disk.
//
// approvedPlan carries the text produced by earlier phases (Discovery and
// Decision). The approved plan is authoritative for the Implementation phase:
// an implementer must follow the plan the human authorized at the gate.
func PromptFor(wi *domain.WorkItem, phase domain.Phase, approvedPlan string) string {
	var b strings.Builder
	switch phase {
	case domain.PhaseDiscovery:
		b.WriteString("You are in the DISCOVERY phase of an orchestrated WorkItem.\n")
		b.WriteString("Explore the repository and produce a concise discovery report: relevant files, symbols, dependencies, and conventions.\n")
		b.WriteString("Do not modify any files in this phase.\n\n")
	case domain.PhaseDecision:
		b.WriteString("You are in the DECISION phase of an orchestrated WorkItem.\n")
		b.WriteString("Produce a concrete implementation plan: the files to change, the approach, and the test strategy.\n")
		b.WriteString("State explicitly and minimally what the deliverable is.\n")
		b.WriteString("Do not modify any files in this phase.\n\n")
	case domain.PhaseImplementation:
		b.WriteString("You are in the IMPLEMENTATION phase of an orchestrated WorkItem.\n")
		b.WriteString("Implement EXACTLY the approved plan below — no more, no less. Do not expand scope, do not refactor unrelated code.\n")
		b.WriteString("The approved plan is authoritative and overrides any earlier read-only framing in the work item description.\n")
		b.WriteString("Commit your work in this worktree when done.\n")
		if strings.TrimSpace(approvedPlan) != "" {
			b.WriteString("\n=== APPROVED PLAN (authoritative) ===\n")
			b.WriteString(strings.TrimSpace(approvedPlan))
			b.WriteString("\n=== END APPROVED PLAN ===\n")
		}
		b.WriteString("\n")
	case domain.PhaseValidation:
		b.WriteString("You are in the VALIDATION phase of an orchestrated WorkItem.\n")
		b.WriteString("Run the project's tests, linter, and type checker. Report results faithfully.\n")
		b.WriteString("Exit with a non-zero status if any required check fails.\n\n")
	default:
		b.WriteString("Orchestrated WorkItem phase " + string(phase) + ".\n\n")
	}

	b.WriteString("WorkItem: " + string(wi.ID) + "\n")
	b.WriteString("Title: " + wi.Title + "\n")
	if wi.Description != "" {
		b.WriteString("Description: " + wi.Description + "\n")
	}
	b.WriteString(fmt.Sprintf("Base branch: %s\n", wi.BaseBranch))
	return b.String()
}
