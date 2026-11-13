package visual

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/birdnet-go/internal/api/v2/apitest"
	"github.com/tphakala/birdnet-go/internal/conf"
)

func TestFrigateSelectedCameras(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/stats", r.URL.Path)
		_, _ = w.Write([]byte(`{"cameras":{"feeder":{"camera_fps":4,"ffmpeg_pid":123},"private":{"camera_fps":10}},"secret":"hidden"}`))
	}))
	defer server.Close()
	core := apitest.NewCore(t, apitest.WithoutSettingsPublish(), apitest.WithSettingsFunc(func(s *conf.Settings) {
		s.Realtime.Frigate = conf.FrigateSettings{Enabled: true, BaseURL: server.URL, Cameras: []string{"feeder"}, RequestTimeout: 1, MaxAttempts: 1, PollInterval: 20, InitialHistoryHours: 24, CorrelationWindow: 30, OverlapSeconds: 3600, SnapshotRetentionHours: 168}
	}))
	handler := New(core, nil)
	recorder := httptest.NewRecorder()
	ctx := core.Echo.NewContext(httptest.NewRequest(http.MethodGet, "/", http.NoBody), recorder)
	require.NoError(t, handler.cameras(ctx))
	var result struct {
		Enabled bool         `json:"enabled"`
		Cameras []cameraInfo `json:"cameras"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
	require.Len(t, result.Cameras, 1)
	assert.Equal(t, cameraInfo{Name: "feeder", Online: true, FPS: 4}, result.Cameras[0])
	assert.NotContains(t, recorder.Body.String(), "secret")
	assert.NotContains(t, recorder.Body.String(), "private")
	ctx.SetParamNames("camera")
	ctx.SetParamValues("private")
	err := handler.live(ctx)
	require.Error(t, err)
	var httpErr *echo.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.Code)
}
