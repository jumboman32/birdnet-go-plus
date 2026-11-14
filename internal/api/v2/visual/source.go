package visual

import (
	"context"
	"encoding/json"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/tphakala/birdnet-go/internal/frigate"
)

const (
	sourceLifetime       = 10 * time.Minute
	sourceReadTimeout    = 30 * time.Second
	sourceWriteTimeout   = 10 * time.Second
	sourceHeartbeat      = 10 * time.Second
	sourceMaxMessage     = 8 * 1024 * 1024
	sourceMaxClientInput = 1024
	sourceMaxMetadata    = 1024
)

var sourceUpgrader = websocket.Upgrader{CheckOrigin: sourceOriginAllowed}
var sourceMimeCodecs = regexp.MustCompile(`^(avc1\.[0-9a-fA-F]{6}|hvc1\.[a-zA-Z0-9.]+)$`)

func sourceOriginAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // Non-browser clients still pass the route's authentication middleware.
	}
	u, err := url.Parse(origin)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host == r.Host && u.User == nil
}

func (c *Handler) source(ctx echo.Context) error {
	cfg := c.CurrentSettings().Realtime.Frigate
	camera := ctx.Param("camera")
	if !cfg.Enabled || !slices.Contains(cfg.Cameras, camera) {
		return echo.NewHTTPError(http.StatusNotFound, "Frigate camera is not enabled or selected")
	}
	if !sourceOriginAllowed(ctx.Request()) {
		return echo.NewHTTPError(http.StatusForbidden, "Cross-origin source video is not allowed")
	}
	select {
	case c.liveSlots <- struct{}{}:
		defer func() { <-c.liveSlots }()
	default:
		return echo.NewHTTPError(http.StatusTooManyRequests, "Live camera viewer capacity reached")
	}
	client, err := frigate.NewClient(&cfg, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid Frigate connection settings")
	}
	request, cancel := context.WithTimeout(ctx.Request().Context(), sourceLifetime)
	defer cancel()
	upstream, err := client.OpenSource(request, camera, strings.Split(ctx.QueryParam("codecs"), ","))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "Frigate source video is unavailable")
	}
	defer func() { _ = upstream.Close() }()
	downstream, err := sourceUpgrader.Upgrade(ctx.Response(), ctx.Request(), nil)
	if err != nil {
		return nil // The upgrader has written its HTTP error response.
	}
	stop := context.AfterFunc(request, func() { _ = upstream.Close(); _ = downstream.Close() })
	defer stop()
	var workers sync.WaitGroup
	defer func() {
		cancel()
		_ = downstream.Close()
		_ = upstream.Close()
		workers.Wait()
	}()
	downstream.SetReadLimit(sourceMaxClientInput)
	_ = downstream.SetReadDeadline(time.Now().Add(sourceWriteTimeout))
	// Wait for the browser's MediaSource to open before relaying any fragments.
	// The codec list has already been validated and sent upstream; do not forward
	// this or any later browser message to go2rtc.
	var negotiation struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	}
	if downstream.ReadJSON(&negotiation) != nil || negotiation.Type != "mse" || negotiation.Value != ctx.QueryParam("codecs") {
		return nil
	}
	_ = downstream.SetReadDeadline(time.Now().Add(sourceReadTimeout))
	downstream.SetPongHandler(func(string) error { return downstream.SetReadDeadline(time.Now().Add(sourceReadTimeout)) })
	workers.Go(func() {
		defer cancel()
		// Never forward browser commands to go2rtc. Only protocol control frames are needed.
		_, _, _ = downstream.ReadMessage()
	})
	workers.Go(func() {
		ticker := time.NewTicker(sourceHeartbeat)
		defer ticker.Stop()
		defer cancel()
		for {
			select {
			case <-request.Done():
				return
			case <-c.Context().Done():
				return
			case <-ticker.C:
				current := c.CurrentSettings().Realtime.Frigate
				if !current.Enabled || current.BaseURL != cfg.BaseURL || current.Username != cfg.Username || current.Password != cfg.Password || current.Token != cfg.Token || !slices.Contains(current.Cameras, camera) {
					return
				}
				if downstream.WriteControl(websocket.PingMessage, nil, time.Now().Add(sourceWriteTimeout)) != nil {
					return
				}
			}
		}
	})
	upstream.SetReadLimit(sourceMaxMessage)
	for {
		_ = upstream.SetReadDeadline(time.Now().Add(sourceReadTimeout))
		kind, data, err := upstream.ReadMessage()
		if err != nil {
			return nil
		}
		if kind == websocket.TextMessage {
			var ok bool
			data, ok = sourceMetadata(data)
			if !ok {
				return nil // Raw go2rtc errors may contain source URLs; never expose them.
			}
		}
		_ = downstream.SetWriteDeadline(time.Now().Add(sourceWriteTimeout))
		if downstream.WriteMessage(kind, data) != nil {
			return nil
		}
	}
}

func sourceMetadata(data []byte) ([]byte, bool) {
	if len(data) > sourceMaxMetadata {
		return nil, false
	}
	var message struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	}
	if json.Unmarshal(data, &message) != nil || message.Type != "mse" {
		return nil, false
	}
	mediaType, params, err := mime.ParseMediaType(message.Value)
	if err != nil || mediaType != "video/mp4" || params["codecs"] == "" || len(params) != 1 {
		return nil, false
	}
	for codec := range strings.SplitSeq(params["codecs"], ",") {
		if !sourceMimeCodecs.MatchString(strings.TrimSpace(codec)) {
			return nil, false
		}
	}
	clean, err := json.Marshal(message)
	return clean, err == nil
}
