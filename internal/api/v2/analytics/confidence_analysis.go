package analytics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/tphakala/birdnet-go/internal/api/v2/apicore"
	datastoreV2 "github.com/tphakala/birdnet-go/internal/datastore/v2"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/repository"
	"github.com/tphakala/birdnet-go/internal/wallclock"
)

// getConfidenceAnalysis is an additive representation on the existing distribution endpoint.
// It never uses the legacy store or infers model identity from historical confidence values.
func (c *Handler) getConfidenceAnalysis(ctx echo.Context) error {
	const operation = "confidence analysis"
	if err := c.requireQueryParam(ctx, "start_date", operation); err != nil {
		return err
	}
	startDate, endDate := ctx.QueryParam("start_date"), ctx.QueryParam("end_date")
	if err := c.validateDateFormatStrictWithResponse(ctx, startDate, "start_date", operation); err != nil {
		return err
	}
	if err := c.validateDateFormatStrictWithResponse(ctx, endDate, "end_date", operation); err != nil {
		return err
	}
	if err := c.validateDateRangeWithResponse(ctx, startDate, endDate, operation); err != nil {
		return err
	}
	if c.V2Manager == nil || !datastoreV2.IsEnhancedDatabase() {
		return c.HandleError(ctx, nil, "Confidence analysis requires the enhanced database", http.StatusNotImplemented)
	}
	start, _ := wallclock.ParseDate(startDate)
	if endDate == "" {
		endDate = start.AddDays(defaultAnalyticsDays).String()
	}
	end, _ := wallclock.ParseDate(endDate)
	startTime, _ := wallclock.DayBounds(start, time.Local)
	_, endTime := wallclock.DayBounds(end, time.Local)
	species := ctx.QueryParams()["species"]
	const maxConfidenceFilters = 10
	if len(species) > maxConfidenceFilters {
		return c.HandleError(ctx, nil, "At most 10 species can be compared", http.StatusBadRequest)
	}
	for i, name := range species {
		if resolved, hit := c.resolveSpeciesToScientific(name); hit {
			species[i] = resolved
		}
	}
	models := make([]uint, 0)
	for _, raw := range ctx.QueryParams()["model"] {
		id, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			return c.HandleError(ctx, err, "Invalid model ID", http.StatusBadRequest)
		}
		models = append(models, uint(id))
	}
	if len(models) > maxConfidenceFilters {
		return c.HandleError(ctx, nil, "At most 10 models can be compared", http.StatusBadRequest)
	}
	repo := repository.NewConfidenceAnalyticsRepository(c.V2Manager.DB(), c.V2Manager.TablePrefix() != "")
	requestCtx, cancel := withAnalyticsTimeout(ctx)
	defer cancel()
	if ctx.QueryParam("format") == "review" {
		bin, err := strconv.Atoi(ctx.QueryParam("bin"))
		const binCount = 20
		if err != nil || bin < 0 || bin >= binCount || len(models) != 1 || len(species) > 1 {
			return c.HandleError(ctx, err, "Review requires one model, at most one species, and a bin from 0 to 19", http.StatusBadRequest)
		}
		offset := 0
		if raw := ctx.QueryParam("offset"); raw != "" {
			offset, err = strconv.Atoi(raw)
			if err != nil || offset < 0 {
				return c.HandleError(ctx, err, "Invalid review offset", http.StatusBadRequest)
			}
		}
		name := ""
		if len(species) > 0 {
			name = species[0]
		}
		rows, err := repo.Review(requestCtx, startTime.Unix(), endTime.Unix(), name, models[0], bin, offset)
		if err != nil {
			return c.handleAnalyticsQueryError(ctx, err, operation, "Failed to load confidence review")
		}
		names := c.loadCommonNameMap()
		for i := range rows {
			rows[i].CommonName = apicore.ResolveCommonName(names, rows[i].Species)
		}
		return ctx.JSON(http.StatusOK, rows)
	}
	series, err := repo.Distribution(requestCtx, startTime.Unix(), endTime.Unix(), species, models)
	if err != nil {
		return c.handleAnalyticsQueryError(ctx, err, operation, "Failed to aggregate confidence analysis")
	}
	names := c.loadCommonNameMap()
	for i := range series {
		series[i].CommonName = apicore.ResolveCommonName(names, series[i].Species)
		series[i].TopCommonName = apicore.ResolveCommonName(names, series[i].TopSpecies)
	}
	return ctx.JSON(http.StatusOK, series)
}
