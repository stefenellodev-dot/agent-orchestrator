package service

import (
	"strings"

	"github.com/stefenello/agent-orchestrator/internal/domain"
)

// ParseCapabilities derives the OpenCode CLI surface from verified command
// output. It is deliberately tolerant: anything it cannot confirm is marked
// absent, and it never panics on unexpected input.
func ParseCapabilities(version, runHelp, agentsOutput string) domain.CLICapabilities {
	caps := domain.CLICapabilities{
		Version:         strings.TrimSpace(version),
		SupportsJSON:    hasFlag(runHelp, "--format") && strings.Contains(runHelp, "json"),
		SupportsSession: hasFlag(runHelp, "--session"),
		SupportsContinue: hasFlag(runHelp, "--continue"),
		SupportsFork:    hasFlag(runHelp, "--fork"),
		SupportsAgent:   hasFlag(runHelp, "--agent"),
		SupportsDir:     hasFlag(runHelp, "--dir"),
		SupportsAttach:  hasFlag(runHelp, "--attach"),
	}
	caps.SupportedAgents = parseAgents(agentsOutput)
	return caps
}

// hasFlag reports whether the help text documents the given long flag.
func hasFlag(help, flag string) bool {
	return strings.Contains(help, flag)
}

// parseAgents extracts agent names from `opencode agent list` output. Lines
// look like "build (primary)"; JSON-ish noise lines are ignored.
func parseAgents(out string) []string {
	seen := map[string]bool{}
	var agents []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		if !isAgentToken(name) || seen[name] {
			continue
		}
		seen[name] = true
		agents = append(agents, name)
	}
	return agents
}

// isAgentToken reports whether s looks like an agent name (letters/digits/-/_).
func isAgentToken(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case (r >= '0' && r <= '9') || r == '-' || r == '_':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
