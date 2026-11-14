package frigate

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/entities"
)

type snapshotTransport func(*http.Request) (*http.Response, error)

func (f snapshotTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestFrigateSnapshotBurstWaitsForCapacity(t *testing.T) {
	t.Parallel()
	var jpegData bytes.Buffer
	require.NoError(t, jpeg.Encode(&jpegData, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil))
	cfg := config("http://frigate.example")
	client, err := NewClient(&cfg, snapshotTransport(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(jpegData.Bytes())), Header: make(http.Header)}, nil
	}))
	require.NoError(t, err)
	directory := filepath.Join(t.TempDir(), "snapshots")
	row := &entities.VisualDetection{Instance: InstanceKey(cfg.BaseURL), ExternalEventID: "burst-bird"}
	synctest.Test(t, func(t *testing.T) {
		cache := NewSnapshotCache()
		for range snapshotConcurrency {
			cache.slots <- struct{}{}
		}
		done := make(chan error, 1)
		go func() { _, requestErr := cache.Get(t.Context(), client, directory, row); done <- requestErr }()
		synctest.Wait()
		select {
		case requestErr := <-done:
			require.FailNow(t, "snapshot was rejected instead of queued", "%v", requestErr)
		default:
		}
		<-cache.slots
		require.NoError(t, <-done)
	})
}

func TestFrigateSnapshotQueueHonorsDeadlineAndCancellation(t *testing.T) {
	t.Parallel()
	for _, cancelRequest := range []bool{false, true} {
		t.Run(map[bool]string{false: "deadline", true: "canceled"}[cancelRequest], func(t *testing.T) {
			cfg := config("http://frigate.example")
			client, err := NewClient(&cfg, snapshotTransport(func(_ *http.Request) (*http.Response, error) {
				t.Error("queued request reached network")
				return nil, context.Canceled
			}))
			require.NoError(t, err)
			row := &entities.VisualDetection{Instance: InstanceKey(cfg.BaseURL), ExternalEventID: "queued-bird"}
			directory := t.TempDir()
			synctest.Test(t, func(t *testing.T) {
				cache := NewSnapshotCache()
				for range snapshotConcurrency {
					cache.slots <- struct{}{}
				}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				done := make(chan error, 1)
				go func() { _, requestErr := cache.Get(ctx, client, directory, row); done <- requestErr }()
				synctest.Wait()
				if cancelRequest {
					cancel()
					require.ErrorIs(t, <-done, context.Canceled)
				} else {
					time.Sleep(time.Duration(cfg.RequestTimeout) * time.Second)
					require.ErrorIs(t, <-done, context.DeadlineExceeded)
				}
				assert.Len(t, cache.slots, snapshotConcurrency)
			})
		})
	}
}
