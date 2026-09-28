package api

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/stefenello/agent-orchestrator/internal/buildinfo"
	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/service"
)

// OrchestratorService is the subset of the orchestrator the HTTP layer needs.
type OrchestratorService interface {
	CreateWorkItem(ctx context.Context, in service.CreateWorkItemInput) (*domain.WorkItem, error)
	GetWorkItem(ctx context.Context, id domain.WorkItemID) (*domain.WorkItem, error)
	ListWorkItems(ctx context.Context, project string) ([]*domain.WorkItem, error)
	ListEvents(ctx context.Context, id domain.WorkItemID) ([]*domain.Event, error)
	ListSessions(ctx context.Context, id domain.WorkItemID) ([]*domain.Session, error)

	GetGate(ctx context.Context, id domain.WorkItemID) (*domain.Gate, error)
	Approve(ctx context.Context, id domain.WorkItemID, in service.ApprovalInput) error
	Reject(ctx context.Context, id domain.WorkItemID, in service.ApprovalInput) error
	RequestChanges(ctx context.Context, id domain.WorkItemID, in service.ApprovalInput) error
	Retry(ctx context.Context, id domain.WorkItemID, in service.RetryInput) error
	ProjectList() []service.ProjectConfig
}

// Options configures the HTTP server.
type Options struct {
	AuthEnabled  bool
	AuthUsername string
	AuthPassword string
	StaticDir    string
	// StaticFS, when set, serves the embedded dashboard with SPA fallback.
	StaticFS fs.FS
	// BuildInfo is exposed via /healthz so a stale binary is always visible.
	BuildInfo buildinfo.Info
}

type server struct {
	svc  OrchestratorService
	opts Options
}

// NewServer builds the HTTP handler for the orchestrator API.
func NewServer(svc OrchestratorService, opts Options) http.Handler {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())

	s := &server{svc: svc, opts: opts}

	e.GET("/healthz", s.health)

	api := e.Group("/api")
	if opts.AuthEnabled {
		api.Use(middleware.BasicAuth(func(user, pass string, _ echo.Context) (bool, error) {
			return user == opts.AuthUsername && pass == opts.AuthPassword, nil
		}))
	}
	api.POST("/workitems", s.createWorkItem)
	api.GET("/workitems", s.listWorkItems)
	api.GET("/workitems/:id", s.getWorkItem)
	api.GET("/workitems/:id/events", s.listEvents)
	api.GET("/workitems/:id/sessions", s.listSessions)
	api.GET("/workitems/:id/gate", s.getGate)
	api.POST("/workitems/:id/approve", s.approve)
	api.POST("/workitems/:id/reject", s.reject)
	api.POST("/workitems/:id/request-changes", s.requestChanges)
	api.POST("/workitems/:id/retry", s.retry)
	api.GET("/projects", s.listProjects)

	if opts.StaticDir != "" {
		e.Static("/", opts.StaticDir)
	}
	if opts.StaticFS != nil {
		e.GET("/*", echo.WrapHandler(spaHandler(opts.StaticFS)))
	}
	return e
}

// spaHandler serves static assets and falls back to index.html so client-side
// routes (e.g. #/workitems/<id>) load correctly.
func spaHandler(staticFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(staticFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(staticFS, path); err != nil {
			clone := r.Clone(r.Context())
			clone.URL.Path = "/"
			fileServer.ServeHTTP(w, clone)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func (s *server) health(c echo.Context) error {
	info := s.opts.BuildInfo
	return c.JSON(http.StatusOK, map[string]any{
		"status":     "ok",
		"version":    info.Version,
		"commit":     info.Commit,
		"build_time": info.BuildTime,
		"opencode":   info.OpenCode,
		"agents":     info.Agents,
	})
}

func (s *server) listProjects(c echo.Context) error {
	return c.JSON(http.StatusOK, s.svc.ProjectList())
}

func (s *server) createWorkItem(c echo.Context) error {
	var req createWorkItemRequest
	if err := c.Bind(&req); err != nil {
		return apiError(c, service.ErrInvalidInput)
	}
	wi, err := s.svc.CreateWorkItem(c.Request().Context(), req.toInput())
	if err != nil {
		return apiError(c, err)
	}
	return c.JSON(http.StatusCreated, wi)
}

func (s *server) listWorkItems(c echo.Context) error {
	items, err := s.svc.ListWorkItems(c.Request().Context(), c.QueryParam("project"))
	if err != nil {
		return apiError(c, err)
	}
	if items == nil {
		items = []*domain.WorkItem{}
	}
	return c.JSON(http.StatusOK, items)
}

func (s *server) getWorkItem(c echo.Context) error {
	wi, err := s.svc.GetWorkItem(c.Request().Context(), domain.WorkItemID(c.Param("id")))
	if err != nil {
		return apiError(c, err)
	}
	return c.JSON(http.StatusOK, wi)
}

func (s *server) listEvents(c echo.Context) error {
	events, err := s.svc.ListEvents(c.Request().Context(), domain.WorkItemID(c.Param("id")))
	if err != nil {
		return apiError(c, err)
	}
	if events == nil {
		events = []*domain.Event{}
	}
	return c.JSON(http.StatusOK, events)
}

func (s *server) listSessions(c echo.Context) error {
	sessions, err := s.svc.ListSessions(c.Request().Context(), domain.WorkItemID(c.Param("id")))
	if err != nil {
		return apiError(c, err)
	}
	if sessions == nil {
		sessions = []*domain.Session{}
	}
	return c.JSON(http.StatusOK, sessions)
}

func (s *server) getGate(c echo.Context) error {
	gate, err := s.svc.GetGate(c.Request().Context(), domain.WorkItemID(c.Param("id")))
	if err != nil {
		return apiError(c, err)
	}
	return c.JSON(http.StatusOK, gate)
}

func (s *server) approve(c echo.Context) error { return s.decide(c, s.svc.Approve) }

func (s *server) reject(c echo.Context) error { return s.decide(c, s.svc.Reject) }

func (s *server) requestChanges(c echo.Context) error { return s.decide(c, s.svc.RequestChanges) }

func (s *server) retry(c echo.Context) error {
	var req retryRequest
	if err := c.Bind(&req); err != nil {
		return apiError(c, service.ErrInvalidInput)
	}
	if err := s.svc.Retry(c.Request().Context(), domain.WorkItemID(c.Param("id")), req.toInput()); err != nil {
		return apiError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) decide(c echo.Context, fn func(context.Context, domain.WorkItemID, service.ApprovalInput) error) error {
	var req approvalRequest
	if err := c.Bind(&req); err != nil {
		return apiError(c, service.ErrInvalidInput)
	}
	if err := fn(c.Request().Context(), domain.WorkItemID(c.Param("id")), req.toInput()); err != nil {
		return apiError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// apiError maps domain/service errors to HTTP status codes.
func apiError(c echo.Context, err error) error {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, service.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, service.ErrInvalidInput), errors.Is(err, service.ErrInvalidTransition), errors.Is(err, service.ErrNotRetryable):
		status = http.StatusBadRequest
	case errors.Is(err, service.ErrProjectBusy):
		status = http.StatusConflict
	case errors.Is(err, service.ErrAuthorizationRequired):
		status = http.StatusForbidden
	}
	return c.JSON(status, map[string]string{"error": err.Error()})
}
