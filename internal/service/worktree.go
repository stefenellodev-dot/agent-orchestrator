package service

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Worktree describes a provisioned Git worktree for a WorkItem.
type Worktree struct {
	WorkItemID string
	Path       string
	Branch     string
	BaseBranch string
	// BaseCommitSHA is the immutable commit this worktree branched from.
	BaseCommitSHA string
}

// WorktreeManager owns the worktree lifecycle. One WorkItem owns exactly one
// worktree for its entire lifecycle (Discovery through Validation); it is
// never recreated between phases.
type WorktreeManager interface {
	// Create is idempotent: calling it again for the same WorkItem returns the
	// existing worktree rather than creating a second one.
	Create(ctx context.Context, workItemID, repoPath, baseBranch string) (*Worktree, error)
	// Cleanup removes the worktree. Called only when retention policy allows.
	Cleanup(ctx context.Context, workItemID string) error
	// Path returns the on-disk path for a WorkItem without creating anything.
	Path(workItemID string) string
	// List returns the paths of all git worktrees currently under the root, for
	// orphan detection / garbage collection.
	List() ([]string, error)
}

// GitWorktreeManager provisions one Git worktree per WorkItem under a root
// directory and reuses it for the WorkItem's entire lifecycle.
type GitWorktreeManager struct {
	root string
}

func NewGitWorktreeManager(root string) *GitWorktreeManager {
	return &GitWorktreeManager{root: root}
}

// Path returns the deterministic on-disk location for a WorkItem's worktree.
func (m *GitWorktreeManager) Path(workItemID string) string {
	return filepath.Join(m.root, workItemID)
}

// Create provisions the WorkItem's worktree if it does not already exist.
// It is idempotent: repeated calls return the same worktree.
func (m *GitWorktreeManager) Create(ctx context.Context, workItemID, repoPath, baseBranch string) (*Worktree, error) {
	if workItemID == "" {
		return nil, fmt.Errorf("workItemID is required")
	}
	if repoPath == "" {
		return nil, fmt.Errorf("repoPath is required")
	}
	if baseBranch == "" {
		baseBranch = "main"
	}

	path := m.Path(workItemID)
	wt := &Worktree{WorkItemID: workItemID, Path: path, Branch: branchNameFor(workItemID), BaseBranch: baseBranch}

	if isWorktree(path) {
		wt.BaseCommitSHA = resolveBaseSHA(ctx, path, baseBranch)
		return wt, nil
	}

	if err := os.MkdirAll(m.root, 0o755); err != nil {
		return nil, fmt.Errorf("create worktree root: %w", err)
	}
	// Clear any stale git metadata for worktrees whose directories are gone.
	_, _ = runGitCommand(ctx, repoPath, "worktree", "prune")

	branch := branchNameFor(workItemID)
	var err error
	if branchExists(ctx, repoPath, branch) {
		_, err = runGitCommand(ctx, repoPath, "worktree", "add", path, branch)
	} else {
		_, err = runGitCommand(ctx, repoPath, "worktree", "add", "-b", branch, path, baseBranch)
	}
	if err != nil {
		return nil, fmt.Errorf("git worktree add: %w", err)
	}
	if !isWorktree(path) {
		return nil, fmt.Errorf("worktree at %s was not created correctly", path)
	}
	// The branch was just created from baseBranch, so HEAD is the immutable base.
	wt.BaseCommitSHA = resolveBaseSHA(ctx, path, baseBranch)
	return wt, nil
}

// resolveBaseSHA returns the immutable commit the worktree branched from: the
// merge-base with the base branch (the branch point). This is stable even after
// the base branch advances.
func resolveBaseSHA(ctx context.Context, worktreePath, baseBranch string) string {
	if baseBranch != "" {
		if out, err := runGitCommand(ctx, worktreePath, "merge-base", baseBranch, "HEAD"); err == nil {
			if sha := strings.TrimSpace(out); sha != "" {
				return sha
			}
		}
	}
	if head, err := revParse(ctx, worktreePath); err == nil {
		return head
	}
	return ""
}

// Cleanup removes the WorkItem's worktree. Missing worktrees are not an error.
func (m *GitWorktreeManager) Cleanup(ctx context.Context, workItemID string) error {
	path := m.Path(workItemID)
	if !isWorktree(path) {
		// Nothing of ours to remove. Prune metadata if the directory is gone.
		return nil
	}
	repo, err := mainRepoFor(path)
	if err != nil {
		return fmt.Errorf("resolve main repo for %s: %w", path, err)
	}
	// Safety: never treat the main repository as the worktree to remove.
	if filepath.Clean(repo) == filepath.Clean(path) {
		return fmt.Errorf("refusing to remove main repository %s", repo)
	}
	if _, err := runGitCommand(ctx, repo, "worktree", "remove", "--force", path); err != nil {
		return fmt.Errorf("git worktree remove: %w", err)
	}
	// Best-effort: drop the WorkItem's dedicated branch.
	_, _ = runGitCommand(ctx, repo, "branch", "-D", branchNameFor(workItemID))
	_, _ = runGitCommand(ctx, repo, "worktree", "prune")
	return nil
}

// List returns the paths of all git worktrees currently under the root. Only
// directories that are real linked worktrees are returned; anything else is
// ignored (never a candidate for cleanup).
func (m *GitWorktreeManager) List() ([]string, error) {
	entries, err := os.ReadDir(m.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(m.root, e.Name())
		if isWorktree(p) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

const worktreeBranchPrefix = "workitem/"

func branchNameFor(workItemID string) string {
	return worktreeBranchPrefix + workItemID
}

func branchExists(ctx context.Context, repoPath, branch string) bool {
	_, err := runGitCommand(ctx, repoPath, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// gitCommitAll stages and commits every change in a worktree, returning whether
// a commit was created. An explicit identity is supplied so commits succeed even
// when the repository has no user configured.
func gitCommitAll(ctx context.Context, worktreePath, message string) (bool, error) {
	status, err := runGitCommand(ctx, worktreePath, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(status) == "" {
		return false, nil
	}
	if _, err := runGitCommand(ctx, worktreePath, "add", "-A"); err != nil {
		return false, err
	}
	cmd := exec.CommandContext(ctx, "git",
		"-c", "user.email=orchestrator@localhost",
		"-c", "user.name=orchestrator",
		"commit", "-m", message)
	cmd.Dir = worktreePath
	if out, err := cmd.CombinedOutput(); err != nil {
		return false, fmt.Errorf("git commit: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return true, nil
}

func runGitCommand(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// isWorktree reports whether path exists and is a linked git worktree (its
// .git is a file pointing at the main repository).
func isWorktree(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// mainRepoFor derives the main repository path from a linked worktree by
// reading its .git file: "gitdir: <main>/.git/worktrees/<name>".
func mainRepoFor(worktreePath string) (string, error) {
	data, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf("unexpected .git file contents: %q", line)
	}
	gitdir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	// <main>/.git/worktrees/<name> -> <main>
	return filepath.Dir(filepath.Dir(filepath.Dir(gitdir))), nil
}
