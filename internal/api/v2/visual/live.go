package visual

import (
	"context"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"slices"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/tphakala/birdnet-go/internal/api/v2/apicore"
	"github.com/tphakala/birdnet-go/internal/frigate"
)

const (
	maxLiveViewers = 2
	liveDuration   = 30 * time.Second
)

type cameraInfo struct {
	Name   string  `json:"name"`
	Online bool    `json:"online"`
	FPS    float64 `json:"fps"`
}

func (c *Handler) cameras(ctx echo.Context) error {
	cfg := c.CurrentSettings().Realtime.Frigate
	result := struct {
		Enabled bool         `json:"enabled"`
		Cameras []cameraInfo `json:"cameras"`
	}{Enabled: cfg.Enabled, Cameras: make([]cameraInfo, 0, len(cfg.Cameras))}
	if !cfg.Enabled {
		return ctx.JSON(http.StatusOK, result)
	}
	client, err := frigate.NewClient(&cfg, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	request, cancel := context.WithTimeout(ctx.Request().Context(), time.Duration(cfg.RequestTimeout)*time.Second)
	defer cancel()
	rates, err := client.CameraFrameRates(request)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "Cannot reach Frigate camera statistics; check integration settings")
	}
	for _, name := range cfg.Cameras {
		fps := rates[name]
		result.Cameras = append(result.Cameras, cameraInfo{Name: name, Online: fps > 0, FPS: fps})
	}
	return ctx.JSON(http.StatusOK, result)
}

func (c *Handler) live(ctx echo.Context) error {
	cfg := c.CurrentSettings().Realtime.Frigate
	if !cfg.Enabled {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Frigate integration is disabled")
	}
	camera := ctx.Param("camera")
	if !slices.Contains(cfg.Cameras, camera) {
		return echo.NewHTTPError(http.StatusNotFound, "Camera is not selected in Frigate settings")
	}
	select {
	case c.liveSlots <- struct{}{}:
		defer func() { <-c.liveSlots }()
	default:
		return echo.NewHTTPError(http.StatusTooManyRequests, "Live camera viewer capacity reached")
	}
	client, err := frigate.NewClient(&cfg, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	request, cancel := context.WithTimeout(ctx.Request().Context(), liveDuration)
	defer cancel()
	writer := multipart.NewWriter(ctx.Response().Writer)
	started := false
	err = client.Stream(request, camera, func(frame []byte) error {
		apicore.SetStreamWriteDeadline(ctx)
		if !started {
			ctx.Response().Header().Set("Content-Type", mime.FormatMediaType("multipart/x-mixed-replace", map[string]string{"boundary": writer.Boundary()}))
			ctx.Response().Header().Set("Cache-Control", "private, no-store")
			ctx.Response().Header().Set("X-Content-Type-Options", "nosniff")
			apicore.DisableProxyBuffering(ctx)
			ctx.Response().WriteHeader(http.StatusOK)
			started = true
		}
		part, writeErr := writer.CreatePart(textproto.MIMEHeader{"Content-Type": {"image/jpeg"}, "Content-Length": {strconv.Itoa(len(frame))}})
		if writeErr != nil {
			return writeErr
		}
		if _, writeErr = part.Write(frame); writeErr != nil {
			return writeErr
		}
		ctx.Response().Flush()
		return nil
	})
	if started {
		_ = writer.Close()
		return nil
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "Camera live view is unavailable; check Frigate and the camera stream")
	}
	return echo.NewHTTPError(http.StatusBadGateway, "Frigate returned no live camera frames")
}
