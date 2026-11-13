// Package visual exposes authenticated camera observations and Frigate controls.
package visual

import (
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/tphakala/birdnet-go/internal/api/v2/apicore"
	"github.com/tphakala/birdnet-go/internal/conf"
	datastoreV2 "github.com/tphakala/birdnet-go/internal/datastore/v2"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/repository"
	"github.com/tphakala/birdnet-go/internal/errors"
	"github.com/tphakala/birdnet-go/internal/frigate"
	"github.com/tphakala/birdnet-go/internal/speciesindex"
	"github.com/tphakala/birdnet-go/internal/wallclock"
	"gorm.io/gorm"
)

const (
	defaultLimit   = 50
	maxLimit       = 200
	maxQueryLength = 200
	cacheDirectory = "frigate-snapshots"
)

// Handler owns the visual API and its lifecycle-tracked serial importer.
type Handler struct {
	*apicore.Core
	repo      repository.VisualRepository
	audio     repository.DetectionRepository
	labels    repository.LabelRepository
	service   *frigate.Service
	snapshots *frigate.SnapshotCache
	names     func() *speciesindex.Snapshot
	liveSlots chan struct{}
}

// New builds a handler; repository wiring waits until functional options are applied.
func New(core *apicore.Core, names func() *speciesindex.Snapshot) *Handler {
	return &Handler{Core: core, names: names, snapshots: frigate.NewSnapshotCache(), liveSlots: make(chan struct{}, maxLiveViewers)}
}

// RegisterRoutes wires authenticated routes and starts a worker only on v2 storage.
func (c *Handler) RegisterRoutes(g *echo.Group) {
	if c.V2Manager != nil && datastoreV2.IsEnhancedDatabase() && c.V2Manager.DB() != nil {
		c.repo = repository.NewVisualRepository(c.V2Manager.DB(), c.V2Manager.TablePrefix())
		prefixed := c.V2Manager.TablePrefix() != ""
		c.audio = repository.NewDetectionRepository(c.V2Manager.DB(), nil, prefixed, c.V2Manager.IsMySQL())
		c.labels = repository.NewLabelRepository(c.V2Manager.DB(), nil, prefixed, c.V2Manager.IsMySQL())
		c.service = frigate.NewService(c.repo, func() conf.FrigateSettings { return c.CurrentSettings().Realtime.Frigate }, c.names, nil, c.APILogger)
		c.GoUnlessStopped(func() { c.service.Run(c.Context()) })
		c.GoUnlessStopped(func() { c.cleanupSnapshots() })
	}
	group := g.Group("/visual-detections", c.AuthMiddleware)
	group.GET("", c.list)
	group.GET("/summary", c.summary)
	group.GET("/:id", c.get)
	group.GET("/:id/snapshot", c.snapshot)
	group.GET("/:id/correlations", c.correlations)
	integration := g.Group("/integrations/frigate", c.AuthMiddleware)
	integration.GET("/status", c.status)
	integration.POST("/test", c.test)
	integration.GET("/cameras", c.cameras)
	integration.GET("/cameras/:camera/live", c.live)
}

func (c *Handler) available() error {
	if c.repo == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Visual observations require the v2 database")
	}
	return nil
}

func (c *Handler) status(ctx echo.Context) error {
	if err := c.available(); err != nil {
		return err
	}
	status := c.service.Status()
	_, total, err := c.repo.List(ctx.Request().Context(), &repository.VisualFilter{Limit: 1})
	if err != nil {
		return c.HandleError(ctx, err, "Cannot read visual status", http.StatusInternalServerError)
	}
	status.Imported = total
	return ctx.JSON(http.StatusOK, status)
}

func (c *Handler) test(ctx echo.Context) error {
	client, err := frigate.NewClient(&c.CurrentSettings().Realtime.Frigate, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err = client.TestConnection(ctx.Request().Context()); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	return ctx.JSON(http.StatusOK, map[string]any{"success": true, "message": "Frigate events API is accessible"})
}

func (c *Handler) list(ctx echo.Context) error {
	if err := c.available(); err != nil {
		return err
	}
	f, err := parseFilter(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	rows, total, err := c.repo.List(ctx.Request().Context(), &f)
	if err != nil {
		return c.HandleError(ctx, err, "Cannot list visual observations", http.StatusInternalServerError)
	}
	return ctx.JSON(http.StatusOK, map[string]any{"data": rows, "total": total, "limit": f.Limit, "offset": f.Offset})
}

func parseID(ctx echo.Context) (uint, error) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, strconv.IntSize)
	if err != nil || id == 0 {
		return 0, echo.NewHTTPError(http.StatusBadRequest, "Invalid visual detection ID")
	}
	return uint(id), nil
}

func (c *Handler) get(ctx echo.Context) error {
	if err := c.available(); err != nil {
		return err
	}
	id, err := parseID(ctx)
	if err != nil {
		return err
	}
	row, err := c.repo.Get(ctx.Request().Context(), id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "Visual observation not found")
	}
	if err != nil {
		return c.HandleError(ctx, err, "Cannot read visual observation", http.StatusInternalServerError)
	}
	return ctx.JSON(http.StatusOK, row)
}

func (c *Handler) summary(ctx echo.Context) error {
	if err := c.available(); err != nil {
		return err
	}
	start, end := wallclock.DayBounds(wallclock.DateOf(time.Now().In(time.Local)), time.Local)
	result, err := c.repo.Summary(ctx.Request().Context(), start, end)
	if err != nil {
		return c.HandleError(ctx, err, "Cannot summarize visual observations", http.StatusInternalServerError)
	}
	return ctx.JSON(http.StatusOK, result)
}

func (c *Handler) snapshot(ctx echo.Context) error {
	if err := c.available(); err != nil {
		return err
	}
	id, err := parseID(ctx)
	if err != nil {
		return err
	}
	row, err := c.repo.Get(ctx.Request().Context(), id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "Visual observation not found")
	}
	if err != nil {
		return c.HandleError(ctx, err, "Cannot read visual observation", http.StatusInternalServerError)
	}
	settings := c.CurrentSettings()
	directory, pathErr := snapshotDirectory()
	if pathErr != nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Snapshot cache directory is unavailable")
	}
	client, err := frigate.NewClient(&settings.Realtime.Frigate, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	b, err := c.snapshots.Get(ctx.Request().Context(), client, directory, row)
	if remote, ok := errors.AsType[*frigate.HTTPError](err); ok && (remote.Status == http.StatusNotFound || remote.Status == http.StatusGone) {
		return echo.NewHTTPError(http.StatusNotFound, "Frigate snapshot has expired or is unavailable")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "Snapshot is unavailable; check Frigate connection and snapshot settings")
	}
	ctx.Response().Header().Set("Cache-Control", "private, no-store")
	ctx.Response().Header().Set("X-Content-Type-Options", "nosniff")
	return ctx.Blob(http.StatusOK, "image/jpeg", b)
}

func (c *Handler) cleanupSnapshots() {
	const interval = time.Hour
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		settings := c.CurrentSettings()
		directory, pathErr := snapshotDirectory()
		if pathErr == nil {
			if err := c.snapshots.Cleanup(c.Context(), directory, time.Duration(max(settings.Realtime.Frigate.SnapshotRetentionHours, 1))*time.Hour); err != nil {
				c.LogInfoIfEnabled("Frigate snapshot cleanup failed")
			}
		}
		select {
		case <-c.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func snapshotDirectory() (string, error) {
	base, err := conf.ResolveConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, cacheDirectory), nil
}

func parseFilter(ctx echo.Context) (repository.VisualFilter, error) {
	f := repository.VisualFilter{Camera: ctx.QueryParam("camera"), Species: ctx.QueryParam("species"), Query: strings.TrimSpace(ctx.QueryParam("q")), Limit: defaultLimit}
	if len(f.Query) > maxQueryLength {
		return f, errors.NewStd("Visual search query is too long")
	}
	for key, dest := range map[string]*int{"limit": &f.Limit, "offset": &f.Offset} {
		if value := ctx.QueryParam(key); value != "" {
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 {
				return f, errors.NewStd("Invalid pagination")
			}
			*dest = n
		}
	}
	if f.Limit < 1 || f.Limit > maxLimit {
		return f, errors.NewStd("Limit must be between 1 and 200")
	}
	for key, dest := range map[string]**time.Time{"start": &f.Start, "end": &f.End} {
		if value := ctx.QueryParam(key); value != "" {
			v, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return f, errors.NewStd("Date filters require RFC3339 timestamps")
			}
			v = v.UTC()
			*dest = &v
		}
	}
	if f.Start != nil && f.End != nil && !f.Start.Before(*f.End) {
		return f, errors.NewStd("Start must precede end")
	}
	for key, dest := range map[string]**bool{"identified": &f.Identified, "snapshot": &f.Snapshot} {
		if value := ctx.QueryParam(key); value != "" {
			v, err := strconv.ParseBool(value)
			if err != nil {
				return f, errors.NewStd("Invalid boolean filter")
			}
			*dest = &v
		}
	}
	if value := ctx.QueryParam("minConfidence"); value != "" {
		v, err := strconv.ParseFloat(value, 64)
		if err != nil || !(v >= 0 && v <= 1) {
			return f, errors.NewStd("Confidence must be between zero and one")
		}
		f.MinConfidence = &v
	}
	return f, nil
}
