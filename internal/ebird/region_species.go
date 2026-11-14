package ebird

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"

	"github.com/tphakala/birdnet-go/internal/errors"
)

const maxRegionSpeciesLists = 128

var regionCodePattern = regexp.MustCompile(`^[A-Z]{2}(?:-[A-Za-z0-9]{1,12}){0,2}$`)

// ValidRegionCode reports whether a region is safe to use in an eBird region path.
func ValidRegionCode(region string) bool { return regionCodePattern.MatchString(region) }

// GetRegionSpecies returns a cached copy of eBird species codes ever recorded in a region.
// It uses the all-history species list, never the limited recent-observations feed.
func (c *Client) GetRegionSpecies(ctx context.Context, region string) ([]string, error) {
	if !ValidRegionCode(region) {
		return nil, errors.NewStd("invalid eBird region code")
	}
	codes, err := c.regionSpecies.GetOrLoad(ctx, region, func(loadCtx context.Context) ([]string, error) {
		requestCtx, cancel := context.WithTimeout(loadCtx, c.config.Timeout)
		defer cancel()
		var result []string
		endpoint := fmt.Sprintf("%s/product/spplist/%s", c.config.BaseURL, url.PathEscape(region))
		if err := c.doRequestWithRetry(requestCtx, http.MethodGet, endpoint, nil, &result); err != nil {
			return nil, err
		}
		return result, nil
	})
	return slices.Clone(codes), err
}
