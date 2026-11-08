package species

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/labstack/echo/v4"
)

const (
	calendarDataEnvironment = "BIRDNET_LOCAL_CALENDAR_DATA"
	calendarImportMaxBytes  = 2 << 20
	calendarImportVersion   = 1
)

// GetCalendar serves a private seasonal-report snapshot without reading or
// modifying detections. The administrator supplies its path; callers cannot
// select files. The route uses the same authentication as detection reads.
func (c *Handler) GetCalendar(ctx echo.Context) error {
	ctx.Response().Header().Set("Cache-Control", "private, no-store")
	path := os.Getenv(calendarDataEnvironment)
	if path == "" {
		return c.HandleError(ctx, nil, "Calendar data has not been imported", http.StatusNotFound)
	}
	file, err := os.Open(path)
	if err != nil {
		return c.HandleError(ctx, nil, "Calendar import is unavailable", http.StatusServiceUnavailable)
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, calendarImportMaxBytes+1))
	if err != nil || len(data) > calendarImportMaxBytes {
		return c.HandleError(ctx, nil, "Calendar import could not be read", http.StatusServiceUnavailable)
	}
	var header struct {
		Version   int               `json:"version"`
		Year      int               `json:"year"`
		Generated string            `json:"generated"`
		Species   []json.RawMessage `json:"species"`
	}
	if err := json.Unmarshal(data, &header); err != nil || header.Version != calendarImportVersion || header.Year <= 0 || header.Species == nil {
		return c.HandleError(ctx, nil, "Calendar import is invalid", http.StatusServiceUnavailable)
	}
	if _, err := time.Parse(time.RFC3339, header.Generated); err != nil {
		return c.HandleError(ctx, nil, "Calendar import has an invalid snapshot date", http.StatusServiceUnavailable)
	}
	return ctx.Blob(http.StatusOK, echo.MIMEApplicationJSON, data)
}
