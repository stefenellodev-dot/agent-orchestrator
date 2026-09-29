package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/stefenello/agent-orchestrator/internal/domain"
)

// Diagnostic status/severity values.
const (
	StatusOK   = "ok"
	StatusWarn = "warn"
	StatusFail = "fail"

	SeverityInfo    = "info"
	SeverityWarning = "warning"
	SeverityError   = "error"

	OverallOK        = "ok"
	OverallDegraded  = "degraded"
	OverallUnhealthy = "unhealthy"
)

// Finding is one structured diagnostic result. It explains a check, its status
// and severity, what was found, the affected entity, an observable cause and an
// operational recommendation. Findings never trigger remediation.
type Finding struct {
	Check          string    `json:"check"`
	Status         string    `json:"status"`   // ok | warn | fail
	Severity       string    `json:"severity"` // info | warning | error
	Finding        string    `json:"finding"`
	WorkItemID     string    `json:"work_item_id,omitempty"`
	SessionID      string    `json:"session_id,omitempty"`
	Worktree       string    `json:"worktree,omitempty"`
	Cause          string    `json:"cause,omitempty"`
	Recommendation string    `json:"recommendation,omitempty"`
	At             time.Time `json:"at"`
}

// DiagnosticsReport is a read-only snapshot of the Orchestrator's operational
// state. It never mutates WorkItems, Sessions, worktrees or repositories.
type DiagnosticsReport struct {
	At         time.Time `json:"at"`
	DurationMS int64     `json:"duration_ms"`
	Overall    string    `json:"overall"` // ok | degraded | unhealthy
	Findings   []Finding `json:"findings"`
}

// Diagnostics runs a set of small, deterministic, read-only checks and returns a
// structured report. Remediation stays with R1 (reconciliation) and R2
// (worktree GC); diagnostics only observes.
func (o *Orchestrator) Diagnostics(ctx context.Context) DiagnosticsReport {
	start := o.now()
	var findings []Finding
	add := func(f Finding) {
		f.At = o.now()
		findings = append(findings, f)
	}

	// 1. Database connectivity (safe read).
	items, dbErr := o.store.ListWorkItems(ctx, "")
	if dbErr != nil {
		add(Finding{
			Check: "database", Status: StatusFail, Severity: SeverityError,
			Finding: "database unavailable", Cause: dbErr.Error(),
			Recommendation: "check PostgreSQL connectivity/configuration",
		})
	} else {
		add(Finding{
			Check: "database", Status: StatusOK, Severity: SeverityInfo,
			Finding: fmt.Sprintf("database reachable (%d work items)", len(items)),
		})
	}

	// 2. Runtime (OpenCode) availability (capability probe, bounded).
	if o.adapter == nil {
		add(Finding{Check: "worker", Status: StatusFail, Severity: SeverityError,
			Finding: "runtime adapter not configured"})
	} else {
		pctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		caps, werr := o.adapter.Available(pctx)
		cancel()
		if werr != nil {
			severity := SeverityError
			rec := "check the runtime binary/mount in the worker"
			if errors.Is(werr, domain.ErrRuntimeUnavailable) {
				rec = "runtime unavailable: check the OpenCode binary/mount in the worker"
			}
			add(Finding{Check: "worker", Status: StatusFail, Severity: severity,
				Finding: fmt.Sprintf("%s runtime unavailable", o.adapter.Name()), Cause: werr.Error(),
				Recommendation: rec})
		} else {
			add(Finding{Check: "worker", Status: StatusOK, Severity: SeverityInfo,
				Finding: fmt.Sprintf("%s runtime available (version %s)", o.adapter.Name(), caps.Version)})
		}
	}

	// 3/4. WorkItem + Session consistency (only when the store is reachable).
	if dbErr == nil {
		for _, wi := range items {
			sessions, err := o.store.ListSessions(ctx, wi.ID)
			if err != nil {
				continue
			}
			findings = append(findings, o.diagnoseWorkItem(wi, sessions)...)
		}
	}

	// 5. Worktree ownership/lifecycle (reuses R2 classification; read-only).
	if gc, err := o.ClassifyWorktrees(ctx); err != nil {
		add(Finding{Check: "worktree", Status: StatusFail, Severity: SeverityError,
			Finding: "worktree classification failed", Cause: err.Error()})
	} else {
		for _, p := range gc.Unknown {
			add(Finding{Check: "worktree", Status: StatusWarn, Severity: SeverityWarning,
				Finding: "worktree with unknown ownership", Worktree: p,
				Recommendation: "inspect manually; R2 never auto-deletes unknown worktrees"})
		}
		if dbErr == nil {
			for _, wi := range items {
				if wi.CurrentPhase.IsTerminal() || wi.WorktreePath == "" {
					continue
				}
				if _, statErr := os.Stat(wi.WorktreePath); statErr != nil {
					add(Finding{Check: "worktree", Status: StatusWarn, Severity: SeverityWarning,
						Finding: "worktree expected but missing", WorkItemID: string(wi.ID), Worktree: wi.WorktreePath,
						Recommendation: "inspect; the WorkItem may not be able to run"})
				}
			}
		}
	}

	report := DiagnosticsReport{At: start, Findings: findings}
	report.Overall = overallOf(findings)
	report.DurationMS = time.Since(start).Milliseconds()
	return report
}

// diagnoseWorkItem checks WorkItem/Session consistency for a single WorkItem.
func (o *Orchestrator) diagnoseWorkItem(wi *domain.WorkItem, sessions []*domain.Session) []Finding {
	var out []Finding
	now := o.now()

	var running []*domain.Session
	for _, s := range sessions {
		if s.Status == domain.SessionRunning || s.Status == domain.SessionPending {
			running = append(running, s)
		}
	}

	if wi.CurrentPhase.IsTerminal() {
		for _, s := range running {
			out = append(out, Finding{
				Check: "sessions", Status: StatusWarn, Severity: SeverityWarning,
				Finding:    "terminal WorkItem has an active session",
				WorkItemID: string(wi.ID), SessionID: string(s.ID),
				Cause:          "R1 reconciles orphaned sessions at startup",
				Recommendation: "a restart reconciles it; otherwise inspect",
			})
		}
		return out
	}

	if !wi.CurrentPhase.RequiresOpenCodeRun() {
		return out // e.g. awaiting_approval: legitimately paused, no execution expected
	}

	hasPhaseSession := false
	for _, s := range sessions {
		if s.Phase == wi.CurrentPhase {
			hasPhaseSession = true
			break
		}
	}
	if len(running) == 0 && !hasPhaseSession {
		out = append(out, Finding{
			Check: "workitems", Status: StatusWarn, Severity: SeverityWarning,
			Finding:    "non-terminal WorkItem has no execution evidence for its current phase",
			WorkItemID: string(wi.ID), Cause: "phase " + string(wi.CurrentPhase),
			Recommendation: "R1 resumes on restart; verify the driver is running",
		})
	}
	for _, s := range running {
		if now.Sub(s.StartedAt) > o.phaseTimeout {
			out = append(out, Finding{
				Check: "sessions", Status: StatusWarn, Severity: SeverityWarning,
				Finding:    "session running longer than the phase timeout",
				WorkItemID: string(wi.ID), SessionID: string(s.ID),
				Cause:          fmt.Sprintf("running for %s", now.Sub(s.StartedAt).Round(time.Second)),
				Recommendation: "possibly stuck; R1 reconciles it at startup",
			})
		}
	}
	return out
}

func overallOf(findings []Finding) string {
	overall := OverallOK
	for _, f := range findings {
		switch f.Status {
		case StatusFail:
			return OverallUnhealthy
		case StatusWarn:
			overall = OverallDegraded
		}
	}
	return overall
}
