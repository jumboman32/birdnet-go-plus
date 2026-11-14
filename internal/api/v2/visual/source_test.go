package visual

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/birdnet-go/internal/api/v2/apitest"
	"github.com/tphakala/birdnet-go/internal/conf"
)

func TestFrigateSourceOriginAndMetadata(t *testing.T) {
	t.Parallel()
	for _, origin := range []string{"http://birdnet:8082", "https://birdnet:8082", ""} {
		r := httptest.NewRequest(http.MethodGet, "http://birdnet:8082/source", http.NoBody)
		r.Header.Set("Origin", origin)
		assert.True(t, sourceOriginAllowed(r))
	}
	for _, origin := range []string{"null", "https://evil.test", "http://birdnet:8083", "http://user:password@birdnet:8082"} {
		r := httptest.NewRequest(http.MethodGet, "http://birdnet:8082/source", http.NoBody)
		r.Header.Set("Origin", origin)
		assert.False(t, sourceOriginAllowed(r))
	}
	clean, ok := sourceMetadata([]byte(`{"type":"mse","value":"video/mp4; codecs=avc1.640029","private":"rtsp://user:password@camera"}`))
	require.True(t, ok)
	assert.NotContains(t, string(clean), "private")
	assert.NotContains(t, string(clean), "password")
	for _, raw := range []string{
		`{"type":"error","value":"rtsp://private-source"}`,
		`{"type":"mse","value":"text/html"}`,
		`{"type":"mse","value":"video/mp4; codecs=mp4a.40.2"}`,
		`{"type":"mse","value":"video/mp4; codecs=\"rtsp://user:password@camera\""}`,
		strings.Repeat("x", sourceMaxMetadata+1),
	} {
		_, ok := sourceMetadata([]byte(raw))
		assert.False(t, ok)
	}
}

func TestFrigateSourceProxyAndCleanup(t *testing.T) {
	t.Parallel()
	upgrader := websocket.Upgrader{}
	upstreamClosed := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/config" {
			_, _ = w.Write([]byte(`{"go2rtc":{"streams":{"feeder":[]}}}`))
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		defer func() { _ = conn.Close(); close(upstreamClosed) }()
		var negotiation map[string]string
		if !assert.NoError(t, conn.ReadJSON(&negotiation)) {
			return
		}
		assert.NoError(t, conn.WriteJSON(map[string]string{"type": "mse", "value": "video/mp4; codecs=avc1.640029"}))
		assert.NoError(t, conn.WriteMessage(websocket.BinaryMessage, []byte("original-video-fragment")))
		_, _, _ = conn.ReadMessage()
	}))
	defer upstream.Close()
	core := apitest.NewCore(t, apitest.WithoutSettingsPublish(), apitest.WithSettingsFunc(func(s *conf.Settings) {
		s.Realtime.Frigate = conf.FrigateSettings{Enabled: true, BaseURL: upstream.URL, Cameras: []string{"feeder"}, PollInterval: 20, RequestTimeout: 1, MaxAttempts: 1, InitialHistoryHours: 24, OverlapSeconds: 3600, CorrelationWindow: 30, SnapshotRetentionHours: 168}
	}))
	handler := New(core, nil)
	core.Echo.GET("/source/:camera", handler.source)
	server := httptest.NewServer(core.Echo)
	defer server.Close()
	conn, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/source/feeder?codecs=avc1.640029", http.Header{"Origin": {server.URL}})
	if response != nil {
		_ = response.Body.Close()
	}
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	require.NoError(t, conn.WriteJSON(map[string]string{"type": "mse", "value": "avc1.640029"}))
	_, metadata, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(metadata), "avc1.640029")
	kind, data, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.BinaryMessage, kind)
	assert.Equal(t, "original-video-fragment", string(data))
	require.NoError(t, conn.Close())
	select {
	case <-upstreamClosed:
	case <-time.After(sourceWriteTimeout):
		require.FailNow(t, "Source upstream did not close after the browser disconnected")
	}
	require.Eventually(t, func() bool { return len(handler.liveSlots) == 0 }, sourceWriteTimeout, time.Millisecond)
	request := httptest.NewRequest(http.MethodGet, "/source/feeder?codecs=avc1.640029", http.NoBody)
	request.Header.Set("Origin", "https://evil.test")
	responseRecorder := httptest.NewRecorder()
	core.Echo.ServeHTTP(responseRecorder, request)
	assert.Equal(t, http.StatusForbidden, responseRecorder.Code)
	for range maxLiveViewers {
		handler.liveSlots <- struct{}{}
	}
	responseRecorder = httptest.NewRecorder()
	core.Echo.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/source/feeder", http.NoBody))
	assert.Equal(t, http.StatusTooManyRequests, responseRecorder.Code)
}
