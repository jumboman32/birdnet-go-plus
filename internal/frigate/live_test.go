package frigate

import (
	"bytes"
	"image"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFrigateLiveFrames(t *testing.T) {
	t.Parallel()
	var jpegData bytes.Buffer
	require.NoError(t, jpeg.Encode(&jpegData, image.NewRGBA(image.Rect(0, 0, 16, 16)), nil))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/feeder", r.URL.Path)
		assert.Equal(t, "3", r.URL.Query().Get("fps"))
		cookie, err := r.Cookie("frigate_token")
		assert.NoError(t, err)
		if cookie != nil {
			assert.Equal(t, "secret", cookie.Value)
		}
		writer := multipart.NewWriter(w)
		w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary="+writer.Boundary())
		part, err := writer.CreatePart(textproto.MIMEHeader{"Content-Type": {"image/jpeg"}})
		if assert.NoError(t, err) {
			_, err = part.Write(jpegData.Bytes())
			assert.NoError(t, err)
		}
		assert.NoError(t, writer.Close())
	}))
	defer server.Close()
	cfg := config(server.URL + "/")
	cfg.Token = "secret"
	client, err := NewClient(&cfg, nil)
	require.NoError(t, err)
	count := 0
	require.NoError(t, client.Stream(t.Context(), "feeder", func(frame []byte) error { count++; assert.Equal(t, jpegData.Bytes(), frame); return nil }))
	assert.Equal(t, 1, count)
	for _, name := range []string{"other", "../feeder", "feeder?token=x"} {
		require.Error(t, client.Stream(t.Context(), name, func([]byte) error { t.Error("unexpected frame"); return nil }))
	}
}

func TestFrigateLiveRejectsInvalidMedia(t *testing.T) {
	t.Parallel()
	for _, contentType := range []string{"text/html", "multipart/x-mixed-replace; boundary=frame"} {
		t.Run(contentType, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", contentType)
				_, _ = w.Write([]byte("--frame\r\nContent-Type: image/jpeg\r\n\r\nnot a jpeg\r\n--frame--\r\n"))
			}))
			defer server.Close()
			cfg := config(server.URL)
			client, err := NewClient(&cfg, nil)
			require.NoError(t, err)
			require.Error(t, client.Stream(t.Context(), "feeder", func([]byte) error { t.Error("invalid frame delivered"); return nil }))
		})
	}
}
