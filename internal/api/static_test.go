package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"

	"github.com/stefenello/agent-orchestrator/internal/api"
)

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
