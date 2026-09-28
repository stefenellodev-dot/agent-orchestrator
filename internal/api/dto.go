package api

import (
	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/service"
)

type createWorkItemRequest struct {
	Project     string         `json:"project"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Priority    string         `json:"priority"`
	BaseBranch  string         `json:"base_branch"`
	RepoPath    string         `json:"repo_path"`
	Assignee    string         `json:"assignee"`
	Metadata    map[string]any `json:"metadata"`
}

func (r createWorkItemRequest) toInput() service.CreateWorkItemInput {
	return service.CreateWorkItemInput{
		Project:     r.Project,
		Title:       r.Title,
		Description: r.Description,
		Priority:    domainPriority(r.Priority),
		BaseBranch:  r.BaseBranch,
		RepoPath:    r.RepoPath,
		Assignee:    r.Assignee,
		Metadata:    r.Metadata,
	}
}

// domainPriority validates an incoming priority string, returning "" (which
// the service defaults) when unrecognized.
func domainPriority(p string) domain.Priority {
	switch domain.Priority(p) {
	case domain.PriorityLow, domain.PriorityMedium, domain.PriorityHigh, domain.PriorityCritical:
		return domain.Priority(p)
	default:
		return ""
	}
}

type approvalRequest struct {
	User    string `json:"user"`
	Comment string `json:"comment"`
}

func (r approvalRequest) toInput() service.ApprovalInput {
	user := r.User
	if user == "" {
		user = "unknown"
	}
	return service.ApprovalInput{User: user, Comment: r.Comment}
}
