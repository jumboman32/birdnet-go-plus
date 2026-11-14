package visual

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/tphakala/birdnet-go/internal/errors"
	"gorm.io/gorm"
)

func (c *Handler) reviewSpecies(ctx echo.Context) error {
	if err := c.available(); err != nil {
		return err
	}
	id, err := parseID(ctx)
	if err != nil {
		return err
	}
	var input struct {
		ScientificName string `json:"scientificName"`
		Verified       *bool  `json:"verified"`
	}
	if err = ctx.Bind(&input); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid species review")
	}
	verified := input.Verified == nil || *input.Verified
	scientificName := strings.TrimSpace(input.ScientificName)
	commonName := ""
	if verified {
		if c.names == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "Species index is unavailable")
		}
		names := c.names()
		if names == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "Species index is unavailable")
		}
		if _, exists := names.LabelBySci[scientificName]; !exists {
			return echo.NewHTTPError(http.StatusBadRequest, "Scientific name is not in the species index")
		}
		commonName = names.SciToCommon[scientificName]
	}
	if err = c.repo.ReviewSpecies(ctx.Request().Context(), id, scientificName, commonName, verified); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "Visual observation not found")
		}
		return c.HandleError(ctx, err, "Cannot save species review", http.StatusInternalServerError)
	}
	return ctx.NoContent(http.StatusNoContent)
}
