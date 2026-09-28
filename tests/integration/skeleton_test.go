package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefenello/agent-orchestrator/internal/api"
	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/service"
	"github.com/stefenello/agent-orchestrator/internal/store"
	"github.com/stefenello/agent-orchestrator/internal/store/memory"
	"github.com/stefenello/agent-orchestrator/internal/store/postgres"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	base := []string{"-c", "user.email=test@example.com", "-c", "user.name=Test", "-c", "commit.gpgsign=false"}
	cmd := exec.Command("git", append(base, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v: %s", args, string(out))
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o600))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")
	return dir
}

func withFakeOpenCode(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then echo "1.18.31"; exit 0; fi
if [ "$1" = "agent" ] && [ "$2" = "list" ]; then echo "build (primary)"; echo "explore (subagent)"; exit 0; fi
if [ "$1" = "run" ] && [ "$2" = "--help" ]; then
  printf '%s\n' '--format  default or json' '--session' '--continue' '--fork' '--agent' '--dir' '--attach'
  exit 0
fi
echo "FAKE_RUN_OUTPUT"
exit 0
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "opencode"), []byte(script), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestSkeleton_WorkItemReachesComplete(t *testing.T) {
	runSkeleton(t, "memory", memory.New())
}

func TestSkeleton_WorkItemReachesComplete_Postgres(t *testing.T) {
	dsn := os.Getenv("ORCHESTRATOR_TEST_DSN")
	if dsn == "" {
		t.Skip("ORCHESTRATOR_TEST_DSN not set; skipping PostgreSQL skeleton")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	_, err = pool.Exec(context.Background(), `DROP TABLE IF EXISTS approvals, gates, events, sessions, work_items, schema_migrations CASCADE;`)
	require.NoError(t, err)
	require.NoError(t, postgres.Migrate(context.Background(), pool))

	runSkeleton(t, "postgres", postgres.NewStore(pool))
}

func runSkeleton(t *testing.T, name string, st store.Store) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		ctx := context.Background()
		repo := initRepo(t)
		root := t.TempDir()
		withFakeOpenCode(t)

		orch := service.New(st, service.NewGitWorktreeManager(root), service.NewCLIAdapter("opencode"))
		orch.SetAutoDrive(true)
		orch.SetAutoApprove(true)

		srv := httptest.NewServer(api.NewServer(orch, api.Options{}))
		defer srv.Close()

		body := `{"project":"skeleton-` + name + `","title":"Do the thing","description":"desc","repo_path":"` + repo + `"}`
		resp, err := http.Post(srv.URL+"/api/workitems", "application/json", strings.NewReader(body))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var created domain.WorkItem
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
		require.NotEmpty(t, created.WorktreePath)

		final := pollUntil(t, srv.URL+"/api/workitems/"+string(created.ID), 10*time.Second, func(wi domain.WorkItem) bool {
			return wi.CurrentPhase.IsTerminal()
		})
		assert.Equal(t, domain.PhaseComplete, final.CurrentPhase)

		events := fetchEvents(t, srv.URL+"/api/workitems/"+string(created.ID)+"/events")
		var started []string
		for _, e := range events {
			if e.Type == domain.EventPhaseStarted {
				if p, ok := e.Payload["phase"].(string); ok {
					started = append(started, p)
				}
			}
		}
		assert.Equal(t, []string{
			string(domain.PhaseDiscovery),
			string(domain.PhaseDecision),
			string(domain.PhaseAwaitingApproval),
			string(domain.PhaseImplementation),
			string(domain.PhaseValidation),
			string(domain.PhaseComplete),
		}, started)

		require.Eventually(t, func() bool {
			_, err := os.Stat(created.WorktreePath)
			return os.IsNotExist(err)
		}, 5*time.Second, 50*time.Millisecond, "worktree should be cleaned after complete")

		_ = ctx
	})
}

func pollUntil(t *testing.T, url string, timeout time.Duration, done func(domain.WorkItem) bool) domain.WorkItem {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last domain.WorkItem
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		require.NoError(t, err)
		var wi domain.WorkItem
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&wi))
		resp.Body.Close()
		last = wi
		if done(wi) {
			return wi
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting; last phase = %s", last.CurrentPhase)
	return last
}

func fetchEvents(t *testing.T, url string) []domain.Event {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	var events []domain.Event
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&events))
	return events
}
