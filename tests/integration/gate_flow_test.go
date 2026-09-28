package integration_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefenello/agent-orchestrator/internal/api"
	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/service"
	"github.com/stefenello/agent-orchestrator/internal/store/memory"
)

func TestGateFlow_BlocksUntilHumanApproves(t *testing.T) {
	repo := initRepo(t)
	root := t.TempDir()
	withFakeOpenCode(t)

	// autoDrive on, autoApprove OFF: the workflow must pause at the gate.
	orch := service.New(memory.New(), service.NewGitWorktreeManager(root), service.NewCLIAdapter("opencode"))
	orch.SetAutoDrive(true)

	srv := httptest.NewServer(api.NewServer(orch, api.Options{}))
	defer srv.Close()

	body := `{"project":"gateflow","title":"Gated work","repo_path":"` + repo + `"}`
	resp, err := http.Post(srv.URL+"/api/workitems", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created domain.WorkItem
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))

	// It must stop at awaiting_approval and stay there.
	atGate := pollUntil(t, srv.URL+"/api/workitems/"+string(created.ID), 10*time.Second, func(wi domain.WorkItem) bool {
		return wi.CurrentPhase == domain.PhaseAwaitingApproval
	})
	assert.Equal(t, domain.PhaseAwaitingApproval, atGate.CurrentPhase)

	time.Sleep(200 * time.Millisecond)
	stillAtGate := getWorkItem(t, srv.URL+"/api/workitems/"+string(created.ID))
	assert.Equal(t, domain.PhaseAwaitingApproval, stillAtGate.CurrentPhase, "must not advance without authorization")

	// A gate must exist and be pending.
	gate := getGate(t, srv.URL+"/api/workitems/"+string(created.ID)+"/gate")
	assert.Equal(t, domain.GatePending, gate.Status)

	// Human approves.
	approveResp, err := http.Post(srv.URL+"/api/workitems/"+string(created.ID)+"/approve",
		"application/json", strings.NewReader(`{"user":"alice","comment":"lgtm"}`))
	require.NoError(t, err)
	approveResp.Body.Close()
	require.Equal(t, http.StatusOK, approveResp.StatusCode)

	// Now it should run to completion.
	final := pollUntil(t, srv.URL+"/api/workitems/"+string(created.ID), 10*time.Second, func(wi domain.WorkItem) bool {
		return wi.CurrentPhase.IsTerminal()
	})
	assert.Equal(t, domain.PhaseComplete, final.CurrentPhase)

	approvedGate := getGate(t, srv.URL+"/api/workitems/"+string(created.ID)+"/gate")
	assert.Equal(t, domain.GateApproved, approvedGate.Status)
	require.Len(t, approvedGate.Approvals, 1)
	require.NotNil(t, approvedGate.Approvals[0].Authorization)
	assert.True(t, approvedGate.Approvals[0].Authorization.Granted)

	events := fetchEvents(t, srv.URL+"/api/workitems/"+string(created.ID)+"/events")
	assert.True(t, hasEvent(events, domain.EventAuthorizationGranted))
}

func getWorkItem(t *testing.T, url string) domain.WorkItem {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	var wi domain.WorkItem
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&wi))
	return wi
}

func getGate(t *testing.T, url string) domain.Gate {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	var g domain.Gate
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&g))
	return g
}

func hasEvent(events []domain.Event, typ domain.EventType) bool {
	for _, e := range events {
		if e.Type == typ {
			return true
		}
	}
	return false
}
