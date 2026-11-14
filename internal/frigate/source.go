package frigate

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tphakala/birdnet-go/internal/errors"
)

const sourceHandshakeType = "mse"

var sourceVideoCodecs = []string{"avc1.640029", "avc1.64002A", "avc1.640033", "hvc1.1.6.L153.B0"}

// OpenSource opens a video-only go2rtc MSE stream for a selected camera. Only
// configured stream names and supported codec identifiers can reach go2rtc;
// arbitrary source URLs, commands and browser credentials are never forwarded.
func (c *Client) OpenSource(ctx context.Context, camera string, codecs []string) (*websocket.Conn, error) {
	if !c.settings.Enabled || !slices.Contains(c.settings.Cameras, camera) || !cameraNamePattern.MatchString(camera) {
		return nil, errors.NewStd("Frigate camera is not enabled or selected")
	}
	if len(codecs) == 0 || len(codecs) > len(sourceVideoCodecs) {
		return nil, errors.NewStd("invalid source video codecs")
	}
	for _, codec := range codecs {
		if !slices.Contains(sourceVideoCodecs, codec) {
			return nil, errors.NewStd("unsupported source video codec")
		}
	}
	stream, err := c.sourceName(ctx, camera)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(c.settings.BaseURL)
	if err != nil {
		return nil, errors.NewStd("invalid Frigate URL")
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/live/mse/api/ws"
	u.RawQuery = url.Values{"src": {stream}}.Encode()
	req := &http.Request{Header: make(http.Header)}
	c.authorize(req)
	dialer := websocket.Dialer{Proxy: http.ProxyFromEnvironment, HandshakeTimeout: time.Duration(c.settings.RequestTimeout) * time.Second}
	conn, response, err := dialer.DialContext(ctx, u.String(), req.Header)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		return nil, errors.NewStd("Frigate source video connection failed")
	}
	if err := conn.SetWriteDeadline(time.Now().Add(time.Duration(c.settings.RequestTimeout) * time.Second)); err != nil {
		_ = conn.Close()
		return nil, errors.NewStd("cannot configure source video handshake")
	}
	if err := conn.WriteJSON(struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	}{sourceHandshakeType, strings.Join(codecs, ",")}); err != nil {
		_ = conn.Close()
		return nil, errors.NewStd("cannot negotiate source video")
	}
	return conn, nil
}

func (c *Client) sourceName(ctx context.Context, camera string) (string, error) {
	b, err := c.request(ctx, "/api/config", nil, maxResponseBytes)
	if err != nil {
		return "", err
	}
	var config struct {
		Cameras map[string]struct {
			Live struct {
				Streams map[string]string `json:"streams"`
			} `json:"live"`
		} `json:"cameras"`
		Go2RTC struct {
			Streams map[string]json.RawMessage `json:"streams"`
		} `json:"go2rtc"`
	}
	if json.Unmarshal(b, &config) != nil {
		return "", errors.NewStd("Frigate returned invalid stream configuration")
	}
	// Prefer the camera-named source; mapped aliases support existing Frigate setups.
	aliases := slices.Collect(maps.Values(config.Cameras[camera].Live.Streams))
	slices.Sort(aliases)
	names := make([]string, 0, 1+len(aliases))
	names = append(names, camera)
	names = append(names, aliases...)
	for _, name := range names {
		if _, exists := config.Go2RTC.Streams[name]; exists && cameraNamePattern.MatchString(name) {
			return name, nil
		}
	}
	return "", errors.NewStd("Frigate camera has no configured go2rtc source")
}
