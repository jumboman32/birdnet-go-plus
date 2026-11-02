package species

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/tphakala/birdnet-go/internal/datastore"
	"github.com/tphakala/birdnet-go/internal/errors"
	"github.com/tphakala/birdnet-go/internal/logger"
	"github.com/tphakala/birdnet-go/internal/rarity"
	"github.com/tphakala/birdnet-go/internal/wallclock"
)

const rarityRequestTimeout = 10 * time.Second

type localRarityResponse struct {
	rarity.Result
	Date           string `json:"date"`
	RecentStatus   string `json:"recentStatus"`
	RecentReported bool   `json:"recentReported"`
	BirdcastURL    string `json:"birdcastUrl"`
}

// GetLocalRarity returns seasonal reporting rarity without writing settings or
// detection data. The import path and eBird credentials remain server-side.
func (c *Handler) GetLocalRarity(ctx echo.Context) error {
	name := strings.TrimSpace(ctx.QueryParam("scientific_name"))
	dateString := ctx.QueryParam("date")
	calendarDate, err := wallclock.ParseDate(dateString)
	if err != nil || name == "" || len(name) > 200 {
		return c.HandleError(ctx, nil, "A scientific_name and ISO date (YYYY-MM-DD) are required", http.StatusBadRequest)
	}
	date := time.Date(calendarDate.Year, calendarDate.Month, calendarDate.Day, 0, 0, 0, 0, time.UTC)
	settings := c.CurrentSettings()
	lat, lon, configured := settings.Location()
	response := localRarityResponse{Result: rarity.Evaluate(nil, name, date, lat, lon), Date: dateString, RecentStatus: "unavailable"}
	if !configured {
		response.Reason = "Configure the station location to evaluate local rarity."
		response.ReasonCode = "location_required"
		return ctx.JSON(http.StatusOK, response)
	}
	region := os.Getenv("BIRDNET_RARITY_REGION")
	if path := os.Getenv("BIRDNET_LOCAL_RARITY_DATA"); path != "" {
		result, loadErr := c.rarityStore.Score(path, time.Now(), name, date, lat, lon)
		if loadErr != nil {
			// Avoid exposing server paths or import contents in the API response.
			response.Reason = "The local rarity import could not be read or validated."
			response.ReasonCode = "invalid_import"
			c.LogWarnIfEnabled("Local rarity import unavailable", logger.String("operation", "load_local_rarity_import"), logger.String("failure_kind", rarityImportFailureKind(loadErr)))
		} else {
			response.Result = result
			// The imported region is meaningful only for the configured station.
			if result.RegionCode != "" {
				region = result.RegionCode
			}
		}
	}
	if validBirdcastRegion(region) {
		response.BirdcastURL = "https://dashboard.birdcast.org/region/" + region + "?night=" + calendarDate.AddDays(-1).String()
	}
	// A recent species-list response is neither a checklist sample nor evidence
	// of absence. Do not use today's sightings to explain historical detections.
	if dateString != time.Now().In(datastore.Timezone()).Format(time.DateOnly) {
		response.RecentStatus = "historical_unavailable"
		return ctx.JSON(http.StatusOK, response)
	}
	if client := c.ebirdClient(); client != nil {
		requestCtx, cancel := context.WithTimeout(ctx.Request().Context(), rarityRequestTimeout)
		defer cancel()
		observations, fetchErr := client.GetRecentObservations(requestCtx, lat, lon, 14)
		if fetchErr == nil {
			response.RecentStatus = "available"
			for _, observation := range observations {
				if observation.ScientificName == name {
					response.RecentReported = true
					break
				}
			}
		}
	}
	return ctx.JSON(http.StatusOK, response)
}

func rarityImportFailureKind(err error) string {
	if errors.Is(err, os.ErrNotExist) {
		return "file_missing"
	}
	if errors.Is(err, os.ErrPermission) {
		return "permission_denied"
	}
	if _, ok := errors.AsType[*json.SyntaxError](err); ok {
		return "invalid_json"
	}
	if _, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		return "invalid_schema"
	}
	return "validation_or_io"
}

func validBirdcastRegion(region string) bool {
	// BirdCast supports US states and counties. Reject paths/query syntax even
	// though imported configuration, rather than browser input, supplies the code.
	if len(region) != 9 || region[:3] != "US-" || region[5] != '-' {
		return false
	}
	for i, c := range region {
		if i == 2 || i == 5 {
			continue
		}
		if i < 5 {
			if c < 'A' || c > 'Z' {
				return false
			}
		} else if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
