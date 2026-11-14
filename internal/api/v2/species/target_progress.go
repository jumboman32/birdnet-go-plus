package species

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	datastoreV2 "github.com/tphakala/birdnet-go/internal/datastore/v2"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/repository"
	"github.com/tphakala/birdnet-go/internal/ebird"
	"golang.org/x/sync/errgroup"
)

const targetProgressTimeout = 20 * time.Second

type targetProgress struct {
	Region     string   `json:"region"`
	Detected   int      `json:"detected"`
	Total      int      `json:"total"`
	Percentage *float64 `json:"percentage"`
}

type targetProgressResponse struct {
	Status string          `json:"status"`
	County *targetProgress `json:"county,omitempty"`
	State  *targetProgress `json:"state,omitempty"`
}

// GetTargetProgress compares the station's lifetime detected species with eBird regional lists.
// User-supplied county codes are read-only filters and never modify server settings.
func (c *Handler) GetTargetProgress(ctx echo.Context) error {
	county := strings.TrimSpace(ctx.QueryParam("county"))
	if county == "" {
		county = strings.TrimSpace(os.Getenv("BIRDNET_RARITY_REGION"))
	}
	if county != "" && !ebird.ValidRegionCode(county) {
		return c.HandleError(ctx, nil, "Invalid eBird county code", http.StatusBadRequest)
	}
	parts := strings.Split(county, "-")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return ctx.JSON(http.StatusOK, targetProgressResponse{Status: "region_required"})
	}
	client := c.ebirdClient()
	if client == nil {
		return ctx.JSON(http.StatusOK, targetProgressResponse{Status: "ebird_required"})
	}
	if c.V2Manager == nil || !datastoreV2.IsEnhancedDatabase() {
		return ctx.JSON(http.StatusOK, targetProgressResponse{Status: "database_required"})
	}
	state := strings.Join(parts[:2], "-")
	requestCtx, cancel := context.WithTimeout(ctx.Request().Context(), targetProgressTimeout)
	defer cancel()
	var countyCodes, stateCodes []string
	var taxonomy []ebird.TaxonomyEntry
	group, fetchCtx := errgroup.WithContext(requestCtx)
	group.Go(func() error { var err error; countyCodes, err = client.GetRegionSpecies(fetchCtx, county); return err })
	group.Go(func() error { var err error; stateCodes, err = client.GetRegionSpecies(fetchCtx, state); return err })
	group.Go(func() error { var err error; taxonomy, err = client.GetTaxonomy(fetchCtx, "en"); return err })
	if err := group.Wait(); err != nil {
		return c.HandleError(ctx, err, "Unable to fetch eBird regional species lists", http.StatusBadGateway)
	}
	repo := repository.NewDetectionRepository(c.V2Manager.DB(), nil, c.V2Manager.TablePrefix() != "", c.V2Manager.IsMySQL())
	observed, err := repo.GetSpeciesSummary(requestCtx, 0, time.Now().Unix()+1, nil)
	if err != nil {
		return c.HandleError(ctx, err, "Unable to count detected station species", http.StatusInternalServerError)
	}
	seen := make(map[string]struct{}, len(observed))
	for _, row := range observed {
		seen[row.ScientificName] = struct{}{}
	}
	countyProgress := calculateTargetProgress(county, countyCodes, taxonomy, seen)
	stateProgress := calculateTargetProgress(state, stateCodes, taxonomy, seen)
	return ctx.JSON(http.StatusOK, targetProgressResponse{Status: "available", County: &countyProgress, State: &stateProgress})
}

func calculateTargetProgress(region string, codes []string, taxonomy []ebird.TaxonomyEntry, seen map[string]struct{}) targetProgress {
	targetCodes := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		targetCodes[code] = struct{}{}
	}
	targetNames := make(map[string]struct{})
	for i := range taxonomy {
		taxon := &taxonomy[i]
		if taxon.Category != "species" || taxon.ScientificName == "" {
			continue
		}
		if _, exists := targetCodes[taxon.SpeciesCode]; exists {
			targetNames[taxon.ScientificName] = struct{}{}
		}
	}
	result := targetProgress{Region: region, Total: len(targetNames)}
	for name := range targetNames {
		if _, found := seen[name]; found {
			result.Detected++
		}
	}
	if result.Total > 0 {
		percentage := float64(result.Detected) / float64(result.Total) * 100
		result.Percentage = &percentage
	}
	return result
}
