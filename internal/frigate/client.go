// Package frigate consumes Frigate observations; it performs no video inference.
package frigate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tphakala/birdnet-go/internal/conf"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/entities"
	"github.com/tphakala/birdnet-go/internal/errors"
)

const (
	pageSize          = 100
	maxResponseBytes  = 4 << 20
	maxSnapshotBytes  = 8 << 20
	maxPixels         = 24000000
	maxFieldLength    = 200
	maxLabelLength    = 500
	maxZones          = 100
	backoffBase       = time.Second
	Source            = "frigate"   // Source identifies visual observations from Frigate.
	SnapshotAvailable = "available" // SnapshotAvailable means Frigate reports a snapshot.
	SnapshotMissing   = "missing"   // SnapshotMissing means no snapshot is reported.
)

// HTTPError describes a remote status without exposing response bodies or URLs.
type HTTPError struct{ Status int }

func (e *HTTPError) Error() string { return fmt.Sprintf("Frigate HTTP %d", e.Status) }

// Client owns a bounded, non-redirecting HTTP client and one operation's settings.
type Client struct {
	settings conf.FrigateSettings
	http     *http.Client
	token    string
}

var eventIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// NewClient validates the configuration and uses the supplied transport (nil uses
// the standard transport). Redirects are rejected to protect credentials.
func NewClient(settings *conf.FrigateSettings, transport http.RoundTripper) (*Client, error) {
	if settings.BaseURL == "" {
		return nil, errors.NewStd("Frigate URL is not configured")
	}
	if err := settings.Validate(); err != nil {
		return nil, err
	}
	return &Client{settings: *settings, http: &http.Client{Transport: transport, Timeout: time.Duration(settings.RequestTimeout) * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (c *Client) request(ctx context.Context, path string, query url.Values, limit int64) ([]byte, error) {
	if c.settings.Username != "" && c.settings.Token == "" && c.token == "" {
		if err := c.login(ctx); err != nil {
			return nil, err
		}
	}
	u, err := url.Parse(strings.TrimRight(c.settings.BaseURL, "/") + path)
	if err != nil {
		return nil, errors.NewStd("invalid Frigate URL")
	}
	u.RawQuery = query.Encode()
	for attempt := range c.settings.MaxAttempts {
		b, requestErr := c.once(ctx, u.String(), limit)
		if requestErr == nil {
			return b, nil
		}
		remote, isRemote := errors.AsType[*HTTPError](requestErr)
		if isRemote && remote.Status != http.StatusTooManyRequests && remote.Status < http.StatusInternalServerError {
			return nil, requestErr
		}
		if attempt == c.settings.MaxAttempts-1 {
			return nil, requestErr
		}
		timer := time.NewTimer(backoffBase * time.Duration(1<<attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, errors.NewStd("Frigate request exhausted retries")
}

func (c *Client) once(ctx context.Context, target string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
	if err != nil {
		return nil, errors.NewStd("cannot create Frigate request")
	}
	c.authorize(req)
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.NewStd("Frigate connection failed or request timed out")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{Status: resp.StatusCode}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, errors.NewStd("Frigate response read failed")
	}
	if int64(len(b)) > limit {
		return nil, errors.NewStd("Frigate response exceeds size limit")
	}
	return b, nil
}

func (c *Client) authorize(req *http.Request) {
	if c.settings.Token != "" {
		req.AddCookie(&http.Cookie{Name: "frigate_token", Value: c.settings.Token})
	} else if c.token != "" {
		req.AddCookie(&http.Cookie{Name: "frigate_token", Value: c.token})
	}
}

// Event is Frigate's transport representation. Unknown properties are ignored.
type Event struct {
	ID            string          `json:"id"`
	Camera        string          `json:"camera"`
	Label         string          `json:"label"`
	SubLabel      json.RawMessage `json:"sub_label"`
	TopScore      *float64        `json:"top_score"`
	Start         float64         `json:"start_time"`
	End           *float64        `json:"end_time"`
	Zones         []string        `json:"zones"`
	HasSnapshot   bool            `json:"has_snapshot"`
	HasClip       bool            `json:"has_clip"`
	FalsePositive *bool           `json:"false_positive"`
	Data          struct {
		TopScore      *float64 `json:"top_score"`
		Score         *float64 `json:"score"`
		SubLabelScore *float64 `json:"sub_label_score"`
	} `json:"data"`
}

// Events reads one stable, bounded page. The fixed upper bound prevents new
// events from shifting offsets while a historical window is being scanned.
func (c *Client) Events(ctx context.Context, after, before time.Time, offset int) ([]Event, error) {
	q := url.Values{"labels": {"bird"}, "cameras": {strings.Join(c.settings.Cameras, ",")}, "limit": {strconv.Itoa(pageSize)}, "offset": {strconv.Itoa(offset)}, "after": {unixString(after)}, "before": {unixString(before)}, "sort": {"date_asc"}, "include_thumbnails": {"0"}}
	b, err := c.request(ctx, "/api/events", q, maxResponseBytes)
	if err != nil {
		return nil, err
	}
	var events []Event
	if err = json.Unmarshal(b, &events); err != nil || events == nil {
		return nil, errors.NewStd("Frigate returned an invalid events response")
	}
	if len(events) > pageSize {
		return nil, errors.NewStd("Frigate ignored event page limit")
	}
	return events, nil
}

func unixString(t time.Time) string {
	return strconv.FormatFloat(float64(t.Unix())+float64(t.Nanosecond())/float64(time.Second), 'f', 6, 64)
}

func score(v *float64) *float64 {
	if v == nil || !(*v >= 0 && *v <= 1) {
		return nil
	}
	return v
}

func subLabel(raw json.RawMessage) (label string, confidence *float64) {
	if json.Unmarshal(raw, &label) == nil {
		return strings.TrimSpace(label), nil
	}
	var tuple []json.RawMessage
	if json.Unmarshal(raw, &tuple) != nil || len(tuple) == 0 {
		return "", nil
	}
	if json.Unmarshal(tuple[0], &label) != nil {
		return "", nil
	}
	if len(tuple) > 1 {
		if json.Unmarshal(tuple[1], &confidence) != nil {
			confidence = nil
		}
	}
	return strings.TrimSpace(label), score(confidence)
}

func timestamp(v float64) (time.Time, error) {
	const maxUnix = 253402300799
	if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || v > maxUnix {
		return time.Time{}, errors.NewStd("invalid Frigate timestamp")
	}
	seconds, fraction := math.Modf(v)
	return time.Unix(int64(seconds), int64(fraction*float64(time.Second))).UTC(), nil
}

// Observation validates an event and preserves missing classification as unknown.
func (e *Event) Observation(instance string) (*entities.VisualDetection, error) {
	if !eventIDPattern.MatchString(e.ID) || len(e.ID) > maxFieldLength || e.Camera == "" || len(e.Camera) > maxFieldLength || e.Label != "bird" || len(e.Zones) > maxZones {
		return nil, errors.NewStd("invalid Frigate bird event identity")
	}
	start, err := timestamp(e.Start)
	if err != nil {
		return nil, err
	}
	row := &entities.VisualDetection{Source: Source, Instance: instance, ExternalEventID: e.ID, Camera: e.Camera, ObjectLabel: e.Label, StartTime: start, Zones: e.Zones, ClipAvailable: e.HasClip, FalsePositive: e.FalsePositive, SnapshotStatus: SnapshotMissing}
	if e.HasSnapshot {
		row.SnapshotStatus = SnapshotAvailable
	}
	if e.End != nil {
		end, endErr := timestamp(*e.End)
		if endErr != nil || end.Before(start) {
			return nil, errors.NewStd("invalid Frigate event end time")
		}
		row.EndTime = &end
	}
	row.DetectionConfidence = score(e.TopScore)
	if row.DetectionConfidence == nil {
		row.DetectionConfidence = score(e.Data.TopScore)
	}
	if row.DetectionConfidence == nil {
		row.DetectionConfidence = score(e.Data.Score)
	}
	label, confidence := subLabel(e.SubLabel)
	if len(label) > maxLabelLength {
		return nil, errors.NewStd("Frigate species label exceeds length limit")
	}
	if label != "" {
		row.OriginalLabel = &label
		row.ClassificationSource = entities.VisualClassificationFrigate
		row.ClassificationConfidence = confidence
		if confidence == nil {
			row.ClassificationConfidence = score(e.Data.SubLabelScore)
		}
	}
	return row, nil
}

// TestConnection verifies that the configured endpoint returns valid event JSON.
func (c *Client) TestConnection(ctx context.Context) error {
	_, err := c.Events(ctx, time.Now().Add(-time.Hour), time.Now(), 0)
	return err
}
