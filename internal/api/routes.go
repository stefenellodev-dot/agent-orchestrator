package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/service"
)

// OrchestratorService is the subset of the orchestrator the HTTP layer needs.
type OrchestratorService interface {
	CreateWorkItem(ctx context.Context, in service.CreateWorkItemInput) (*domain.WorkItem, error)
	GetWorkItem(ctx context.Context, id domain.WorkItemID) (*domain.WorkItem, error)
	ListWorkItems(ctx context.Context, project string) ([]*domain.WorkItem, error)
	ListEvents(ctx context.Context, id domain.WorkItemID) ([]*domain.Event, error)
}

// Options configures the HTTP server.
type Options struct {
	AuthEnabled  bool
	AuthUsername string
	AuthPassword string
	StaticDir    string
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

	if opts.StaticDir != "" {
		e.Static("/", opts.StaticDir)
	}
	return e
}

func (s *server) health(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
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

// apiError maps domain/service errors to HTTP status codes.
func apiError(c echo.Context, err error) error {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, service.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, service.ErrInvalidInput), errors.Is(err, service.ErrInvalidTransition):
		status = http.StatusBadRequest
	case errors.Is(err, service.ErrProjectBusy):
		status = http.StatusConflict
	case errors.Is(err, service.ErrAuthorizationRequired):
		status = http.StatusForbidden
	}
	return c.JSON(status, map[string]string{"error": err.Error()})
}
