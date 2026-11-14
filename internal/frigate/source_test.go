package frigate

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFrigateSourceNegotiatesOnlyVideoForMappedCamera(t *testing.T) {
	t.Parallel()
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/config" {
			_, _ = w.Write([]byte(`{"cameras":{"feeder":{"live":{"streams":{"Main":"feeder_main"}}}},"go2rtc":{"streams":{"feeder_main":["rtsp://private-source"]}}}`))
			return
		}
		assert.Equal(t, "/live/mse/api/ws", r.URL.Path)
		assert.Equal(t, "feeder_main", r.URL.Query().Get("src"))
		cookie, err := r.Cookie("frigate_token")
		if assert.NoError(t, err) {
			assert.Equal(t, "server-secret", cookie.Value)
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		defer func() { _ = conn.Close() }()
		var request struct{ Type, Value string }
		if assert.NoError(t, conn.ReadJSON(&request)) {
			assert.Equal(t, "mse", request.Type)
			assert.Equal(t, "avc1.640029", request.Value)
		}
		assert.NoError(t, conn.WriteMessage(websocket.BinaryMessage, []byte("source-video")))
	}))
	defer server.Close()
	cfg := config(server.URL)
	cfg.Token = "server-secret"
	client, err := NewClient(&cfg, nil)
	require.NoError(t, err)
	conn, err := client.OpenSource(t.Context(), "feeder", []string{"avc1.640029"})
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close()) }()
	kind, data, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.BinaryMessage, kind)
	assert.Equal(t, "source-video", string(data))
	for _, camera := range []string{"other", "../feeder", "ffmpeg:input"} {
		_, err := client.OpenSource(t.Context(), camera, []string{"avc1.640029"})
		require.Error(t, err)
	}
	for _, codecs := range [][]string{nil, {"mp4a.40.2"}, {"ffmpeg:input"}, {"avc1.640029", "user:password"}} {
		_, err := client.OpenSource(t.Context(), "feeder", codecs)
		require.Error(t, err)
	}
}

func TestFrigateSourceRejectsMissingOrDynamicSource(t *testing.T) {
	t.Parallel()
	for _, configJSON := range []string{
		`{"cameras":{},"go2rtc":{"streams":{}}}`,
		`{"cameras":{"feeder":{"live":{"streams":{"Main":"ffmpeg:input"}}}},"go2rtc":{"streams":{"ffmpeg:input":[]}}}`,
		`invalid json`,
	} {
		t.Run(configJSON, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/config", r.URL.Path)
				_, _ = w.Write([]byte(configJSON))
			}))
			defer server.Close()
			cfg := config(server.URL)
			client, err := NewClient(&cfg, nil)
			require.NoError(t, err)
			_, err = client.OpenSource(t.Context(), "feeder", []string{"avc1.640029"})
			require.Error(t, err)
		})
	}
}
