package frigate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tphakala/birdnet-go/internal/conf"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/entities"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/repository"
	"github.com/tphakala/birdnet-go/internal/errors"
	"github.com/tphakala/birdnet-go/internal/logger"
	"github.com/tphakala/birdnet-go/internal/speciesindex"
)

const (
	settingsCheckInterval = time.Second
	syncTimeout           = 2 * time.Minute
	maxPages              = 1000
	maxCatchupWindow      = 12 * time.Hour
)

// Status contains safe diagnostics and basic integration counters.
type Status struct {
	Enabled     bool       `json:"enabled"`
	Connected   bool       `json:"connected"`
	LastSuccess *time.Time `json:"lastSuccessfulImport"`
	LastError   string     `json:"lastError"`
	Imported    int64      `json:"totalImported"`
	Polls       uint64     `json:"polls"`
	Failures    uint64     `json:"failures"`
}

// Service owns one serial polling loop; dependencies are read per operation.
type Service struct {
	repo      repository.VisualRepository
	settings  func() conf.FrigateSettings
	names     func() *speciesindex.Snapshot
	transport http.RoundTripper
	log       logger.Logger
	mu        sync.RWMutex
	status    Status
}

// NewService constructs an importer without starting goroutines.
func NewService(repo repository.VisualRepository, settings func() conf.FrigateSettings, names func() *speciesindex.Snapshot, transport http.RoundTripper, log logger.Logger) *Service {
	return &Service{repo: repo, settings: settings, names: names, transport: transport, log: log}
}

// InstanceKey isolates external IDs when the configured Frigate server changes.
func InstanceKey(baseURL string) string {
	sum := sha256.Sum256([]byte(strings.TrimRight(baseURL, "/")))
	return hex.EncodeToString(sum[:])
}

func cursorKey(cfg *conf.FrigateSettings) string {
	cameras := slices.Clone(cfg.Cameras)
	slices.Sort(cameras)
	return InstanceKey(cfg.BaseURL + "\x00" + strings.Join(cameras, "\x00") + fmt.Sprint(cfg.InitialHistoryHours))
}

// Status returns a consistent copy of the worker's diagnostics.
func (s *Service) Status() Status {
	s.mu.RLock()
	status := s.status
	s.mu.RUnlock()
	status.Enabled = s.settings().Enabled
	if !status.Enabled {
		status.Connected = false
	}
	return status
}

// Run polls serially and observes setting changes within a second while idle.
// Each HTTP and database operation is context bounded; shutdown cancels backoff.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(settingsCheckInterval)
	defer ticker.Stop()
	var next time.Time
	var previous conf.FrigateSettings
	for {
		cfg := s.settings()
		changed := !sameSettings(&cfg, &previous)
		if cfg.Enabled && (changed || !time.Now().Before(next)) {
			err := s.syncLive(ctx, &cfg)
			s.record(err)
			next = time.Now().Add(time.Duration(max(cfg.PollInterval, 1)) * time.Second)
		}
		previous = cfg
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// syncLive cancels an in-flight request on configuration changes and joins it
// before starting another poll. There is at most one synchronization operation.
func (s *Service) syncLive(ctx context.Context, cfg *conf.FrigateSettings) error {
	operation, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Sync(operation, cfg) }()
	ticker := time.NewTicker(settingsCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			cancel()
			return <-done
		case <-ticker.C:
			current := s.settings()
			if !sameSettings(cfg, &current) {
				cancel()
				return <-done
			}
		}
	}
}

func sameSettings(a, b *conf.FrigateSettings) bool {
	return a.Enabled == b.Enabled && a.BaseURL == b.BaseURL && a.Username == b.Username && a.Password == b.Password && a.Token == b.Token && slices.Equal(a.Cameras, b.Cameras) && a.PollInterval == b.PollInterval && a.RequestTimeout == b.RequestTimeout && a.MaxAttempts == b.MaxAttempts && a.InitialHistoryHours == b.InitialHistoryHours && a.OverlapSeconds == b.OverlapSeconds && a.ClassificationThreshold == b.ClassificationThreshold
}

func (s *Service) record(err error) {
	s.mu.Lock()
	s.status.Polls++
	if err != nil {
		s.status.Connected = false
		s.status.Failures++
		s.status.LastError = "Frigate synchronization failed; cursor retained"
		if remote, ok := errors.AsType[*HTTPError](err); ok {
			s.status.LastError = remote.Error()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			s.status.LastError = "Frigate synchronization timed out; cursor retained"
		}
	} else {
		now := time.Now().UTC()
		s.status.Connected = true
		s.status.LastSuccess = &now
		s.status.LastError = ""
	}
	s.mu.Unlock()
	if err != nil && s.log != nil && !errors.Is(err, context.Canceled) {
		s.log.Warn("Frigate synchronization failed; will retry with retained cursor", logger.String("operation", "frigate_sync"))
	}
}

// Sync imports a fixed time window. Pages are idempotently committed before the
// cursor: a crash or failed page safely replays the entire unfinished window.
func (s *Service) Sync(parent context.Context, cfg *conf.FrigateSettings) error {
	if !cfg.Enabled {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, syncTimeout)
	defer cancel()
	client, err := NewClient(cfg, s.transport)
	if err != nil {
		return err
	}
	key := cursorKey(cfg)
	through, err := s.repo.Cursor(ctx, key)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	after := through.Add(-time.Duration(cfg.OverlapSeconds) * time.Second)
	if through.IsZero() {
		through = now.Add(-time.Duration(cfg.InitialHistoryHours) * time.Hour)
		after = through
	}
	before := now
	if limit := through.Add(maxCatchupWindow); limit.Before(before) {
		before = limit
	}
	instance := InstanceKey(cfg.BaseURL)
	for page := range maxPages {
		events, fetchErr := client.Events(ctx, after, before, page*pageSize)
		if fetchErr != nil {
			return fetchErr
		}
		rows := make([]entities.VisualDetection, 0, len(events))
		for i := range events {
			if !slices.Contains(cfg.Cameras, events[i].Camera) {
				return errors.NewStd("Frigate returned an unselected camera")
			}
			row, parseErr := events[i].Observation(instance)
			if parseErr != nil {
				return parseErr
			}
			s.normalize(row, cfg.ClassificationThreshold)
			rows = append(rows, *row)
		}
		if err := s.repo.Upsert(ctx, rows); err != nil {
			return err
		}
		if len(events) < pageSize {
			if err := s.repo.SetCursor(ctx, key, before); err != nil {
				return err
			}
			_, total, countErr := s.repo.List(ctx, &repository.VisualFilter{Limit: 1})
			if countErr != nil {
				return countErr
			}
			s.mu.Lock()
			s.status.Imported = total
			s.mu.Unlock()
			return nil
		}
	}
	return errors.NewStd("Frigate window exceeded page budget; reduce history or overlap")
}

func (s *Service) normalize(row *entities.VisualDetection, threshold float64) {
	if row.OriginalLabel == nil || s.names == nil {
		return
	}
	if row.ClassificationConfidence != nil && *row.ClassificationConfidence < threshold {
		return
	}
	snapshot := s.names()
	if snapshot == nil {
		return
	}
	name := strings.TrimSpace(*row.OriginalLabel)
	sci := snapshot.CommonToSci[speciesindex.Fold(name)]
	if sci == "" {
		if _, ok := snapshot.LabelBySci[name]; ok {
			sci = name
		}
	}
	if sci == "" {
		return
	}
	common := snapshot.SciToCommon[sci]
	row.ScientificName = &sci
	if common != "" {
		row.CommonName = &common
	}
}
