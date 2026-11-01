// Package rarity estimates seasonal reporting rarity from locally supplied eBird
// complete checklists. Radar migration context never changes a species score.
package rarity

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/tphakala/birdnet-go/internal/ttlcache"
	"github.com/tphakala/birdnet-go/internal/wallclock"
)

const (
	minimumSamples      = 100
	minimumYears        = 3
	seasonWindow        = 14
	maximumBytes        = 32 << 20
	cacheLifetime       = time.Minute
	maximumDuration     = 180
	maximumDistance     = 5
	maximumObservers    = 10
	earthRadiusKM       = 6371.0088
	confidenceZ         = 1.96
	maximumCachedScores = 512
)

// Checklist is one independent, complete eBird sampling event. Species are
// scientific names; shared checklists must have the same GroupID.
type Checklist struct {
	ID              string   `json:"id"`
	GroupID         string   `json:"groupId"`
	LocalityID      string   `json:"localityId"`
	Date            string   `json:"date"`
	Latitude        float64  `json:"latitude"`
	Longitude       float64  `json:"longitude"`
	Complete        bool     `json:"complete"`
	Protocol        string   `json:"protocol"`
	DurationMinutes float64  `json:"durationMinutes"`
	DistanceKM      float64  `json:"distanceKm"`
	Observers       int      `json:"observers"`
	Species         []string `json:"species"`
}

// Dataset is the versioned local interchange format. It contains no API keys or
// observer identities. Taxa explicitly defines scientific-name coverage.
type Dataset struct {
	Version     int         `json:"version"`
	Source      string      `json:"source"`
	RetrievedAt string      `json:"retrievedAt"`
	RegionCode  string      `json:"regionCode"`
	RegionName  string      `json:"regionName"`
	Latitude    float64     `json:"latitude"`
	Longitude   float64     `json:"longitude"`
	RadiusKM    float64     `json:"radiusKm"`
	Taxa        []string    `json:"taxa"`
	Checklists  []Checklist `json:"checklists"`
}

// Result explains the score, its denominator, uncertainty, and data provenance.
// A nil Score always means insufficient data, never a rarity of zero.
type Result struct {
	Status      string  `json:"status"`
	ReasonCode  string  `json:"reasonCode"`
	Reason      string  `json:"reason"`
	Score       *int    `json:"score"`
	ScoreLow    *int    `json:"scoreLow"`
	ScoreHigh   *int    `json:"scoreHigh"`
	Samples     int     `json:"samples"`
	Reports     int     `json:"reports"`
	Years       int     `json:"years"`
	RegionName  string  `json:"regionName"`
	RegionCode  string  `json:"regionCode,omitempty"`
	RadiusKM    float64 `json:"radiusKm"`
	Source      string  `json:"source"`
	RetrievedAt string  `json:"retrievedAt"`
	Method      string  `json:"method"`
}

// Store bounds file size and caches parsed imports, including failures. The path
// comes only from server configuration, never a request or browser input.
type Store struct {
	mu      sync.Mutex
	path    string
	expires time.Time
	data    *Dataset
	err     error
	results *ttlcache.Cache[scoreKey, Result]
}

type scoreKey struct {
	name, date string
	lat, lon   float64
}

// Load reads an import at most once per minute, allowing atomic file replacement
// without a restart. Failed refreshes never return stale data as current evidence.
func (s *Store) Load(path string, now time.Time) (*Dataset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if path == s.path && now.Before(s.expires) {
		return s.data, s.err
	}
	s.path, s.expires = path, now.Add(cacheLifetime)
	s.data, s.err = readDataset(path)
	s.results = ttlcache.New[scoreKey, Result](cacheLifetime, ttlcache.WithMaxEntries(maximumCachedScores))
	return s.data, s.err
}

// Score reuses seasonal calculations until the parsed import refreshes. The
// bounded cache includes location and observation date, so neither can leak
// evidence from another station or detection.
func (s *Store) Score(path string, now time.Time, name string, date time.Time, lat, lon float64) (Result, error) {
	d, err := s.Load(path, now)
	if err != nil {
		return Result{}, err
	}
	if !validCoordinates(lat, lon) {
		return Evaluate(d, name, date, lat, lon), nil
	}
	s.mu.Lock()
	cache := s.results
	current := d == s.data
	s.mu.Unlock()
	key := scoreKey{name: name, date: date.Format(time.DateOnly), lat: lat, lon: lon}
	if current {
		if r, ok := cache.Get(key); ok {
			return cloneResult(&r), nil
		}
	}
	r := Evaluate(d, name, date, lat, lon)
	if current {
		cache.Set(key, cloneResult(&r))
	}
	return r, nil
}

func readDataset(path string) (data *Dataset, resultErr error) {
	f, err := os.Open(path) //nolint:gosec // Server-configured import path; never supplied by an HTTP request.
	if err != nil {
		return nil, fmt.Errorf("open rarity import: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil && resultErr == nil {
			data, resultErr = nil, fmt.Errorf("close rarity import: %w", err)
		}
	}()
	b, err := io.ReadAll(io.LimitReader(f, maximumBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read rarity import: %w", err)
	}
	if len(b) > maximumBytes {
		return nil, fmt.Errorf("rarity import exceeds %d bytes", maximumBytes)
	}
	var d Dataset
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("decode rarity import: %w", err)
	}
	if d.Version != 1 || d.Source == "" || d.RegionCode == "" || d.RegionName == "" ||
		d.RadiusKM <= 0 || d.RadiusKM > 50 || !validCoordinates(d.Latitude, d.Longitude) {
		return nil, fmt.Errorf("invalid rarity import metadata")
	}
	if _, err := time.Parse(time.RFC3339, d.RetrievedAt); err != nil {
		return nil, fmt.Errorf("invalid rarity import retrieval date: %w", err)
	}
	return &d, nil
}

// Evaluate estimates reporting rarity within ±14 calendar days of the observation
// in prior years only. It standardizes effort by restriction, not occupancy
// modelling, and retains one event per locality/day to limit repeated sampling.
func Evaluate(d *Dataset, scientificName string, date time.Time, lat, lon float64) Result {
	r := Result{Status: "insufficient_data", ReasonCode: "missing_data", Reason: "Historical complete-checklist data has not been supplied.", Method: "ebird-reporting-v1"}
	if d == nil {
		return r
	}
	r.RegionName, r.RadiusKM, r.Source, r.RetrievedAt = d.RegionName, d.RadiusKM, d.Source, d.RetrievedAt
	if !d.MatchesLocation(lat, lon) {
		r.Reason = "The imported dataset does not match the configured station location."
		r.ReasonCode = "station_mismatch"
		return r
	}
	r.RegionCode = d.RegionCode
	if !slices.Contains(d.Taxa, scientificName) {
		r.Reason = "This species has no verified taxonomy match in the imported data."
		r.ReasonCode = "unmapped_taxon"
		return r
	}
	years := make(map[int]struct{})
	seen := make(map[string]struct{})
	groups := make(map[string]struct{})
	for i := range d.Checklists {
		c := &d.Checklists[i]
		day, err := wallclock.ParseDate(c.Date)
		if err != nil || day.Year >= date.Year() || !eligible(c) ||
			distanceKM(d.Latitude, d.Longitude, c.Latitude, c.Longitude) > d.RadiusKM || !seasonal(day, wallclock.DateOf(date)) {
			continue
		}
		key := c.LocalityID + "/" + c.Date
		if _, ok := seen[key]; ok {
			continue
		}
		if c.GroupID != "" {
			if _, ok := groups[c.GroupID]; ok {
				continue
			}
			groups[c.GroupID] = struct{}{}
		}
		seen[key] = struct{}{}
		years[day.Year] = struct{}{}
		r.Samples++
		if slices.Contains(c.Species, scientificName) {
			r.Reports++
		}
	}
	r.Years = len(years)
	if r.Samples < minimumSamples || r.Years < minimumYears {
		r.Reason = "At least 100 eligible locality-days across 3 prior years are required in the seasonal window."
		r.ReasonCode = "insufficient_samples"
		return r
	}
	n, k := float64(r.Samples), float64(r.Reports)
	// Jeffreys smoothing prevents zero reports from implying certain absence.
	score := int(math.Round(100 * (1 - (k+0.5)/(n+1))))
	p, z2 := k/n, confidenceZ*confidenceZ
	center := (p + z2/(2*n)) / (1 + z2/n)
	half := confidenceZ * math.Sqrt(p*(1-p)/n+z2/(4*n*n)) / (1 + z2/n)
	low, high := int(math.Floor(100*(1-math.Min(1, center+half)))), int(math.Ceil(100*(1-math.Max(0, center-half))))
	r.Status, r.Reason, r.Score, r.ScoreLow, r.ScoreHigh = "available", "Seasonal reporting rarity, separate from identification confidence.", &score, &low, &high
	r.ReasonCode = "available"
	return r
}

// MatchesLocation prevents an import prepared for one station from following a
// station-location setting change. The one-kilometre tolerance allows rounding.
func (d *Dataset) MatchesLocation(lat, lon float64) bool {
	return d != nil && validCoordinates(lat, lon) && distanceKM(lat, lon, d.Latitude, d.Longitude) <= 1
}

func eligible(c *Checklist) bool {
	return c.ID != "" && c.LocalityID != "" && c.Complete && (c.Protocol == "P21" || c.Protocol == "P22") &&
		c.DurationMinutes > 0 && c.DurationMinutes <= maximumDuration && c.DistanceKM >= 0 && c.DistanceKM <= maximumDistance &&
		c.Observers > 0 && c.Observers <= maximumObservers && validCoordinates(c.Latitude, c.Longitude)
}

func seasonal(day, target wallclock.Date) bool {
	// Compare actual calendar days around the seasonal anchor in each sample
	// year, including adjacent anchors for December/January. A February 29 target
	// uses February 28 in years where February has only 28 days.
	for offset := -1; offset <= 1; offset++ {
		anchor := wallclock.Date{Year: day.Year + offset, Month: target.Month, Day: target.Day}
		if anchor.Month == time.February && anchor.Day == 29 {
			anchor = (wallclock.Date{Year: anchor.Year, Month: time.March, Day: 1}).AddDays(-1)
		}
		delta := wallclock.DaysBetween(day, anchor)
		if delta >= -seasonWindow && delta <= seasonWindow {
			return true
		}
	}
	return false
}

func cloneResult(original *Result) Result {
	r := *original
	if r.Score != nil {
		r.Score = new(*r.Score)
	}
	if r.ScoreLow != nil {
		r.ScoreLow = new(*r.ScoreLow)
	}
	if r.ScoreHigh != nil {
		r.ScoreHigh = new(*r.ScoreHigh)
	}
	return r
}

func validCoordinates(lat, lon float64) bool {
	return !math.IsNaN(lat) && !math.IsNaN(lon) && lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}

func distanceKM(lat1, lon1, lat2, lon2 float64) float64 {
	const radians = math.Pi / 180
	a, b := lat1*radians, lat2*radians
	x := math.Pow(math.Sin((b-a)/2), 2) + math.Cos(a)*math.Cos(b)*math.Pow(math.Sin((lon2-lon1)*radians/2), 2)
	return earthRadiusKM * 2 * math.Asin(math.Sqrt(min(1, x)))
}
