package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefenello/agent-orchestrator/internal/api"
	"github.com/stefenello/agent-orchestrator/internal/buildinfo"
)

// Phase 1.4 regression: the running build identity is visible via /healthz.
func TestHealth_ExposesBuildIdentity(t *testing.T) {
	e := api.NewServer(newStubService(), api.Options{
		BuildInfo: buildinfo.Info{Version: "v9.9.9", Commit: "abc1234", BuildTime: "2026-01-01T00:00:00Z", OpenCode: "1.18.31"},
	})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "ok", body["status"])
	assert.Equal(t, "v9.9.9", body["version"])
	assert.Equal(t, "abc1234", body["commit"])
	assert.Equal(t, "1.18.31", body["opencode"])
}

func TestProjects_Endpoint(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/projects", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"name":"p"`)
}

func TestDiagnostics_Endpoint(t *testing.T) {
	e := api.NewServer(newStubService(), api.Options{
		BuildInfo: buildinfo.Info{Version: "v9", Commit: "abc1234", OpenCode: "1.15.12"},
	})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/diagnostics", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"overall":"ok"`)
	assert.Contains(t, rec.Body.String(), `"orchestrator"`)

	// /healthz remains compatible.
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"commit":"abc1234"`)
}

func TestStaticFS_ServesAssetsWithSPAFallback(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<html>app-root</html>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log(1)")},
	}
	e := api.NewServer(newStubService(), api.Options{StaticFS: fsys})

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "app-root")

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/some/client/route", nil))
	assert.Contains(t, rec.Body.String(), "app-root", "unknown route falls back to index.html")

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	assert.Contains(t, rec.Body.String(), "console.log")
}
