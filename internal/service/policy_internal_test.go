package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/store/memory"
)

func TestMatchProtected(t *testing.T) {
	pats := []string{"migrations/", "docs/deployment-piave.md", "*.env"}
	assert.True(t, matchProtected(pats, "migrations/001_initial.sql"), "directory prefix")
	assert.True(t, matchProtected(pats, "docs/deployment-piave.md"), "exact path")
	assert.True(t, matchProtected(pats, "prod.env"), "glob")
	assert.False(t, matchProtected(pats, "internal/service/policy.go"), "unrelated file")
	assert.False(t, matchProtected(pats, "docs/readme.md"), "repeated dir name is not the same dir")
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return string(out)
}

func initRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("x\n"), 0o644))
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "base")
	base := string(git(t, dir, "rev-parse", "HEAD"))
	return dir, base[:len(base)-1]
}

func TestCompletionGuards_ProtectedPathBlocks(t *testing.T) {
	ctx := context.Background()
	repo, base := initRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "migrations"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "migrations", "004.sql"), []byte("-- x\n"), 0o644))
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-qm", "touch migrations")

	o := New(memory.New(), nil, nil)
	o.projects["p"] = ProjectConfig{Name: "p", ProtectedPaths: []string{"migrations/"}}
	wi := &domain.WorkItem{ID: "wi-1", Project: "p", WorktreePath: repo, BaseCommitSHA: base}

	err := o.completionGuards(ctx, wi, &domain.SessionOutput{TestCommands: []domain.CommandResult{{Command: "go test ./...", Passed: true}}})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPolicyViolation)

	events, err := o.store.ListEvents(ctx, wi.ID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, domain.EventPolicyRejected, events[0].Type)
}

func TestCompletionGuards_RequireValidationBlocks(t *testing.T) {
	ctx := context.Background()
	o := New(memory.New(), nil, nil)
	o.projects["p"] = ProjectConfig{Name: "p", RequireValidation: true}
	wi := &domain.WorkItem{ID: "wi-2", Project: "p", WorktreePath: t.TempDir()}

	err := o.completionGuards(ctx, wi, &domain.SessionOutput{})
	assert.ErrorIs(t, err, ErrPolicyViolation, "no validation evidence must block completion")

	err = o.completionGuards(ctx, wi, &domain.SessionOutput{TestCommands: []domain.CommandResult{{Command: "go test ./...", Passed: true}}})
	assert.NoError(t, err, "objective validation evidence satisfies the guard")
}
