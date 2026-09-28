package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefenello/agent-orchestrator/internal/api"
	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/service"
)

func TestGateEndpoints(t *testing.T) {
	svc := newStubService()
	e := api.NewServer(svc, api.Options{})

	wi, err := svc.CreateWorkItem(context.Background(), service.CreateWorkItemInput{Project: "p", Title: "t"})
	require.NoError(t, err)
	id := string(wi.ID)
	svc.gates[wi.ID] = &domain.Gate{ID: domain.NewGateID(), WorkItemID: wi.ID, Status: domain.GatePending}

	// GET gate
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/workitems/"+id+"/gate", nil))
	assert.Equal(t, http.StatusOK, rec.Code)

	// Approve
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/workitems/"+id+"/approve", strings.NewReader(`{"user":"alice","comment":"lgtm"}`))
	req.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "alice", svc.lastApproval.User)
	assert.Equal(t, "lgtm", svc.lastApproval.Comment)

	// Request changes
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/workitems/"+id+"/request-changes", strings.NewReader(`{"user":"bob","comment":"rework"}`))
	req.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "rework", svc.lastApproval.Comment)

	// Reject
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/workitems/"+id+"/reject", strings.NewReader(`{"user":"carol","comment":"no"}`))
	req.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "carol", svc.lastApproval.User)
}

func TestGateEndpoint_NotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/workitems/missing/gate", nil)
	newTestServer().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

var _ = json.Marshal
