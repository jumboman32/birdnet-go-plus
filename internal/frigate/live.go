package frigate

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tphakala/birdnet-go/internal/errors"
)

const (
	liveStreamLifetime = 30 * time.Second
	liveFrameRate      = "3"
	liveFrameHeight    = "720"
	liveMediaType      = "multipart/x-mixed-replace"
)

var cameraNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-][a-zA-Z0-9_.-]*$`)

// CameraFrameRates returns Frigate's current camera frame rates without exposing
// camera configuration, source addresses or authentication credentials.
func (c *Client) CameraFrameRates(ctx context.Context) (map[string]float64, error) {
	b, err := c.request(ctx, "/api/stats", nil, maxResponseBytes)
	if err != nil {
		return nil, err
	}
	var stats struct {
		Cameras map[string]struct {
			FPS float64 `json:"camera_fps"`
		} `json:"cameras"`
	}
	if json.Unmarshal(b, &stats) != nil || stats.Cameras == nil {
		return nil, errors.NewStd("Frigate returned invalid camera statistics")
	}
	rates := make(map[string]float64, len(stats.Cameras))
	for name, camera := range stats.Cameras {
		rates[name] = camera.FPS
	}
	return rates, nil
}

// Stream delivers bounded JPEG frames from a selected camera's documented MJPEG
// endpoint. It neither stores frames nor performs video encoding or inference.
// Callers reconnect after the bounded stream lifetime.
func (c *Client) Stream(ctx context.Context, camera string, consume func([]byte) error) error {
	if !c.settings.Enabled || !slices.Contains(c.settings.Cameras, camera) || !cameraNamePattern.MatchString(camera) {
		return errors.NewStd("Frigate camera is not enabled or selected")
	}
	if c.settings.Username != "" && c.settings.Token == "" && c.token == "" {
		if err := c.login(ctx); err != nil {
			return err
		}
	}
	u, err := url.Parse(c.settings.BaseURL)
	if err != nil {
		return errors.NewStd("invalid Frigate URL")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/" + camera
	u.RawQuery = url.Values{"fps": {liveFrameRate}, "height": {liveFrameHeight}, "bbox": {"1"}, "timestamp": {"1"}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return errors.NewStd("cannot create Frigate live request")
	}
	c.authorize(req)
	client := &http.Client{Transport: c.http.Transport, CheckRedirect: c.http.CheckRedirect, Timeout: liveStreamLifetime}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.NewStd("Frigate live connection failed or timed out")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return &HTTPError{Status: resp.StatusCode}
	}
	mediaType, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType != liveMediaType || params["boundary"] == "" {
		return errors.NewStd("Frigate did not return an MJPEG stream")
	}
	reader := multipart.NewReader(resp.Body, params["boundary"])
	for {
		part, readErr := reader.NextPart()
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return errors.NewStd("Frigate live stream ended")
		}
		frame, frameErr := io.ReadAll(io.LimitReader(part, maxSnapshotBytes+1))
		if frameErr != nil {
			return errors.NewStd("cannot read Frigate live frame")
		}
		if len(frame) > maxSnapshotBytes {
			return errors.NewStd("Frigate live frame exceeds size limit")
		}
		_ = part.Close()
		if err := validJPEGHeader(frame); err != nil {
			return err
		}
		if err := consume(frame); err != nil {
			return err
		}
	}
}
