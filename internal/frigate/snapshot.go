package frigate

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"image/jpeg"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/tphakala/birdnet-go/internal/datastore/v2/entities"
	"github.com/tphakala/birdnet-go/internal/errors"
)

const (
	cachePermissions     = 0o700
	cacheFilePermissions = 0o600
	snapshotConcurrency  = 2
)

// SnapshotCache bounds concurrent downloads and contains cached files in os.Root.
type SnapshotCache struct{ slots chan struct{} }

// NewSnapshotCache constructs a cache with no background goroutines.
func NewSnapshotCache() *SnapshotCache {
	return &SnapshotCache{slots: make(chan struct{}, snapshotConcurrency)}
}

func snapshotName(row *entities.VisualDetection) string {
	sum := sha256.Sum256([]byte(row.Instance + "\x00" + row.ExternalEventID))
	return hex.EncodeToString(sum[:]) + ".jpg"
}

// Get serves a validated JPEG through BirdNET, never an external URL. A cache
// entry expires after retention; active events bypass caching as their image changes.
func (s *SnapshotCache) Get(ctx context.Context, client *Client, directory string, row *entities.VisualDetection) ([]byte, error) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return nil, errors.NewStd("Frigate snapshot capacity reached")
	}
	if !client.settings.Enabled || !client.settings.SnapshotImport {
		return nil, errors.NewStd("Frigate snapshot import is disabled")
	}
	if row.Instance != InstanceKey(client.settings.BaseURL) {
		return nil, errors.NewStd("snapshot belongs to a different Frigate connection")
	}
	if err := os.MkdirAll(directory, cachePermissions); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	name := snapshotName(row)
	if row.EndTime != nil {
		if info, statErr := root.Stat(name); statErr == nil && info.Mode().IsRegular() && info.Size() <= maxSnapshotBytes && time.Since(info.ModTime()) < time.Duration(client.settings.SnapshotRetentionHours)*time.Hour {
			if b, readErr := readCached(root, name); readErr == nil && validJPEG(b) == nil {
				return b, nil
			}
		}
	}
	b, err := client.request(ctx, "/api/events/"+url.PathEscape(row.ExternalEventID)+"/snapshot.jpg", nil, maxSnapshotBytes)
	if err != nil {
		return nil, err
	}
	if err := validJPEG(b); err != nil {
		return nil, err
	}
	if row.EndTime == nil {
		return b, nil
	}
	// A generated name and Root prevent event IDs and symlinks escaping the cache.
	temp := name + rand.Text() + ".tmp"
	if err := root.WriteFile(temp, b, cacheFilePermissions); err != nil {
		return nil, err
	}
	if err = root.Rename(temp, name); err != nil {
		_ = root.Remove(temp)
		return nil, err
	}
	return b, nil
}

func readCached(root *os.Root, name string) ([]byte, error) {
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.NewStd("snapshot cache entry is not a regular file")
	}
	b, err := io.ReadAll(io.LimitReader(f, maxSnapshotBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxSnapshotBytes {
		return nil, errors.NewStd("cached snapshot exceeds size limit")
	}
	return b, nil
}

func validJPEG(b []byte) error {
	if err := validJPEGHeader(b); err != nil {
		return err
	}
	if _, err := jpeg.Decode(bytes.NewReader(b)); err != nil {
		return errors.NewStd("Frigate snapshot is malformed")
	}
	return nil
}

func validJPEGHeader(b []byte) error {
	if http.DetectContentType(b) != "image/jpeg" {
		return errors.NewStd("Frigate snapshot is not JPEG")
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(b))
	if err != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > maxPixels {
		return errors.NewStd("Frigate snapshot has invalid or excessive dimensions")
	}
	return nil
}

// Cleanup removes only expired cache artifacts in the dedicated managed directory.
// It is run by the serial importer, never by an unbounded per-request goroutine.
func (s *SnapshotCache) Cleanup(ctx context.Context, directory string, retention time.Duration) error {
	root, err := os.OpenRoot(directory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	const cleanupBatch = 200
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, readErr := dir.ReadDir(cleanupBatch)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		for _, entry := range entries {
			if entry.Type().IsRegular() && (filepath.Ext(entry.Name()) == ".jpg" || filepath.Ext(entry.Name()) == ".tmp") {
				info, infoErr := entry.Info()
				if infoErr != nil {
					return infoErr
				}
				if time.Since(info.ModTime()) >= retention {
					if err := root.Remove(entry.Name()); err != nil {
						return err
					}
				}
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
	}
}
