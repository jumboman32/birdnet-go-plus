package classifier

import (
	"encoding/json"
	"io"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/tphakala/birdnet-go/internal/conf"
	"github.com/tphakala/birdnet-go/internal/errors"
	"github.com/tphakala/birdnet-go/internal/logger"
)

const (
	localOccurrenceVersion          = 1
	localOccurrenceMaxBytes         = 1 << 20
	localOccurrenceMinimumLists     = 100
	localOccurrenceMinimumReports   = 5
	localOccurrenceDefaultThreshold = 0.01
	localOccurrenceMonths           = 12
	localOccurrenceLatitudeLimit    = 90
	localOccurrenceLongitudeLimit   = 180
)

type localOccurrenceBounds struct {
	South float64 `json:"south"`
	North float64 `json:"north"`
	West  float64 `json:"west"`
	East  float64 `json:"east"`
}

type localOccurrenceMonth struct {
	Month      int            `json:"month"`
	Checklists int            `json:"checklists"`
	Species    map[string]int `json:"species"`
}

type localOccurrenceProfile struct {
	Version int                    `json:"version"`
	Region  string                 `json:"region"`
	Bounds  localOccurrenceBounds  `json:"bounds"`
	Months  []localOccurrenceMonth `json:"months"`
}

// localOccurrenceCache holds a validated immutable profile. A settings path change,
// file replacement or removal is observed on the next operation, without restart.
// Its lock is independent of native-backend locks and never calls into them.
type localOccurrenceCache struct {
	mu       sync.Mutex
	path     string
	size     int64
	modified time.Time
	profile  *localOccurrenceProfile
	missing  bool
}

func (c *localOccurrenceCache) load(path string) *localOccurrenceProfile {
	if path == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > localOccurrenceMaxBytes {
		if c.path != path || !c.missing {
			// Do not log the private path or any contents from the personal export.
			GetLogger().Warn("Local occurrence profile unavailable; continuing with the standard range filter")
		}
		c.path, c.profile, c.missing = path, nil, true
		return nil
	}
	if c.path == path && !c.missing && c.size == info.Size() && c.modified.Equal(info.ModTime()) {
		return c.profile
	}
	c.path, c.size, c.modified, c.missing = path, info.Size(), info.ModTime(), false
	c.profile = nil
	file, err := os.Open(path)
	if err == nil {
		defer func() { _ = file.Close() }()
		c.profile, err = decodeLocalOccurrence(io.LimitReader(file, localOccurrenceMaxBytes+1))
	}
	if err != nil {
		GetLogger().Warn("Invalid local occurrence profile; continuing with the standard range filter")
		return nil
	}
	GetLogger().Info("Private seasonal occurrence profile loaded", logger.Int("months", len(c.profile.Months)))
	return c.profile
}

func decodeLocalOccurrence(reader io.Reader) (*localOccurrenceProfile, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	profile := &localOccurrenceProfile{}
	if len(data) > localOccurrenceMaxBytes {
		return nil, errors.NewStd("local occurrence profile exceeds size limit")
	}
	if err := json.Unmarshal(data, profile); err != nil {
		return nil, err
	}
	if profile.Version != localOccurrenceVersion || profile.Region == "" || len(profile.Months) != localOccurrenceMonths || !profile.Bounds.valid() {
		return nil, errors.NewStd("invalid local occurrence profile metadata")
	}
	seen := make(map[int]bool, localOccurrenceMonths)
	for _, month := range profile.Months {
		if month.Month < 1 || month.Month > localOccurrenceMonths || seen[month.Month] || month.Checklists < 0 {
			return nil, errors.NewStd("invalid local occurrence month")
		}
		seen[month.Month] = true
		for name, count := range month.Species {
			if len(strings.Fields(name)) != 2 || count < 0 || count > month.Checklists {
				return nil, errors.NewStd("invalid local occurrence species count")
			}
		}
	}
	return profile, nil
}

func (b localOccurrenceBounds) valid() bool {
	for _, coordinate := range []float64{b.South, b.North, b.West, b.East} {
		if math.IsNaN(coordinate) || math.IsInf(coordinate, 0) {
			return false
		}
	}
	return b.South >= -localOccurrenceLatitudeLimit && b.North <= localOccurrenceLatitudeLimit && b.South <= b.North &&
		b.West >= -localOccurrenceLongitudeLimit && b.East <= localOccurrenceLongitudeLimit && b.West <= b.East
}

func (b localOccurrenceBounds) contains(latitude, longitude float64) bool {
	return latitude >= b.South && latitude <= b.North && longitude >= b.West && longitude <= b.East
}

// supplementLocalSpecies admits supported, seasonally recorded species that the
// geomodel omitted. It never removes species, changes a native score, lowers an
// acoustic threshold, overrides an exclusion, or invents classifier vocabulary.
// Added rows are synthetic overrides so county checklist rates are never mistaken
// for calibrated geomodel rarity. No personal checklist data reaches inference.
func (rfs *rangeFilterService) supplementLocalSpecies(scores []SpeciesScore, date time.Time, settings *conf.Settings, state *rangeFilterState, excluder excludeMatcher) []SpeciesScore {
	profile := rfs.localOccurrence.load(settings.BirdNET.RangeFilter.LocalDataPath)
	if profile == nil || !settings.BirdNET.LocationConfigured || !profile.Bounds.contains(settings.BirdNET.Latitude, settings.BirdNET.Longitude) {
		return scores
	}
	if date.IsZero() {
		date = time.Now()
	}
	var month *localOccurrenceMonth
	for i := range profile.Months {
		if profile.Months[i].Month == int(date.Month()) {
			month = &profile.Months[i]
			break
		}
	}
	if month == nil || month.Checklists < localOccurrenceMinimumLists {
		return scores
	}
	threshold := float64(settings.BirdNET.RangeFilter.Threshold)
	if math.IsNaN(threshold) || threshold < 0 || threshold > 1 {
		threshold = localOccurrenceDefaultThreshold
	}
	eligible := make(map[string]float64, len(month.Species))
	for name, reports := range month.Species {
		frequency := float64(reports) / float64(month.Checklists)
		if reports >= localOccurrenceMinimumReports && frequency >= threshold {
			eligible[canonicalSpeciesKey(name)] = frequency
		}
	}
	seen := make(map[string]bool, len(scores))
	for _, score := range scores {
		seen[canonicalSpeciesKey(score.Label)] = true
	}
	addLabel := func(label string) {
		key := canonicalSpeciesKey(label)
		frequency, supported := eligible[key]
		if supported && !seen[key] && !excluder.matches(label) {
			scores = append(scores, SpeciesScore{Label: label, Score: frequency, IsSyntheticOverride: true})
			seen[key] = true
		}
	}
	for _, label := range state.coveredLabels {
		addLabel(label)
	}
	for _, participant := range state.participants {
		for _, label := range participant.labels {
			addLabel(label)
		}
	}
	return scores
}
