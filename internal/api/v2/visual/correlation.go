package visual

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/repository"
	"github.com/tphakala/birdnet-go/internal/detection"
	"github.com/tphakala/birdnet-go/internal/errors"
	"github.com/tphakala/birdnet-go/internal/speciesindex"
	"gorm.io/gorm"
)

const correlationLimit = 100

// potentialMatch retains independent confidence and identifies a candidate only.
type potentialMatch struct {
	AudioID         uint      `json:"audioID"`
	Timestamp       time.Time `json:"timestamp"`
	ScientificName  string    `json:"scientificName"`
	AudioConfidence float64   `json:"audioConfidence"`
	DeltaSeconds    float64   `json:"deltaSeconds"`
}

func (c *Handler) correlations(ctx echo.Context) error {
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
	matches := make([]potentialMatch, 0)
	if row.ScientificName == nil {
		return ctx.JSON(http.StatusOK, map[string]any{"matches": matches, "total": 0, "relationship": "potential"})
	}
	window := time.Duration(c.CurrentSettings().Realtime.Frigate.CorrelationWindow) * time.Second
	if window <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid correlation window")
	}
	names := []string{*row.ScientificName}
	if c.names != nil {
		snap := c.names()
		if snap != nil {
			for _, label := range snap.LabelsByCanonical[speciesindex.CanonicalKey(*row.ScientificName)] {
				names = append(names, detection.ExtractScientificName(label))
			}
		}
	}
	if c.audio == nil || c.labels == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Audio repository unavailable")
	}
	labels, err := c.labels.GetByScientificNames(ctx.Request().Context(), names)
	if err != nil {
		return c.HandleError(ctx, err, "Cannot resolve audio species", http.StatusInternalServerError)
	}
	ids := make([]uint, 0, len(names))
	scientificByID := make(map[uint]string)
	for name, group := range labels {
		for _, label := range group {
			ids = append(ids, label.ID)
			scientificByID[label.ID] = name
		}
	}
	if len(ids) == 0 {
		return ctx.JSON(http.StatusOK, map[string]any{"matches": matches, "total": 0, "relationship": "potential"})
	}
	// Audio detections store whole Unix seconds. Round the inclusive window to
	// those instants; the v2 repository's upper time bound is exclusive.
	start, end := correlationBounds(row.StartTime, window)
	filters := &repository.SearchFilters{LabelIDs: ids, StartTime: &start, EndTime: &end, Limit: correlationLimit, SortBy: repository.SortFieldDetectedAt, SortDesc: true}
	results, total, err := c.audio.Search(ctx.Request().Context(), filters)
	if err != nil {
		return c.HandleError(ctx, err, "Cannot find audio candidates", http.StatusInternalServerError)
	}
	for _, audio := range results {
		timestamp := time.Unix(audio.DetectedAt, 0).UTC()
		matches = append(matches, potentialMatch{AudioID: audio.ID, Timestamp: timestamp, ScientificName: scientificByID[audio.LabelID], AudioConfidence: audio.Confidence, DeltaSeconds: timestamp.Sub(row.StartTime).Seconds()})
	}
	return ctx.JSON(http.StatusOK, map[string]any{"matches": matches, "total": total, "limit": correlationLimit, "relationship": "potential", "windowSeconds": window.Seconds()})
}

func correlationBounds(observed time.Time, window time.Duration) (start, end int64) {
	lower := observed.Add(-window)
	start = lower.Unix()
	if lower.Nanosecond() != 0 {
		start++
	}
	end = observed.Add(window).Unix() + 1
	return start, end
}
