package species

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalendarImportMissingInvalidAndReplaced(t *testing.T) {
	h := rarityHandler(t, true)
	path := filepath.Join(t.TempDir(), "private-calendar.json")
	t.Setenv(calendarDataEnvironment, "")
	request := func() *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		ctx := h.Echo.NewContext(httptest.NewRequest(http.MethodGet, "/api/v2/species/calendar", http.NoBody), recorder)
		require.NoError(t, h.GetCalendar(ctx))
		assert.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
		assert.NotContains(t, recorder.Body.String(), path)
		return recorder
	}
	assert.Equal(t, http.StatusNotFound, request().Code)
	t.Setenv(calendarDataEnvironment, path)
	assert.Equal(t, http.StatusServiceUnavailable, request().Code)
	valid := `{"version":1,"year":2026,"generated":"2026-11-07T19:00:00-05:00","species":[]}`
	require.NoError(t, os.WriteFile(path, []byte(valid), 0o600))
	response := request()
	assert.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, valid, response.Body.String())
	replacement := strings.Replace(valid, `"year":2026`, `"year":2027`, 1)
	require.NoError(t, os.WriteFile(path, []byte(replacement), 0o600))
	assert.JSONEq(t, replacement, request().Body.String(), "file replacement must take effect without restarting")
	for _, invalid := range []string{"broken", "{}", strings.Repeat(" ", calendarImportMaxBytes+1)} {
		require.NoError(t, os.WriteFile(path, []byte(invalid), 0o600))
		assert.Equal(t, http.StatusServiceUnavailable, request().Code)
	}
}

func TestCalendarRouteUsesDetectionAuthentication(t *testing.T) {
	h := rarityHandler(t, true)
	h.AuthMiddleware = func(_ echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			return ctx.NoContent(http.StatusUnauthorized)
		}
	}
	h.RegisterRoutes(h.Group)
	recorder := httptest.NewRecorder()
	h.Echo.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v2/species/calendar", http.NoBody))
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}
