package rarity

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/birdnet-go/internal/wallclock"
)

const testSpecies = "Cyanocitta cristata"

func fixture() *Dataset {
	d := &Dataset{Version: 1, Source: "Synthetic test data", RetrievedAt: "2026-01-01T00:00:00Z", RegionCode: "US-NJ-001", RegionName: "Atlantic County", Latitude: 39.34, Longitude: -74.57, RadiusKM: 25, Taxa: []string{testSpecies}}
	for i := range 120 {
		c := Checklist{ID: fmt.Sprint(i), LocalityID: fmt.Sprint(i), Date: fmt.Sprintf("%d-05-01", 2023+i%3), Latitude: d.Latitude, Longitude: d.Longitude, Complete: true, Protocol: "P21", DurationMinutes: 30, Observers: 1}
		if i%2 == 0 {
			c.Species = []string{testSpecies}
		}
		d.Checklists = append(d.Checklists, c)
	}
	return d
}

func TestEvaluate(t *testing.T) {
	t.Parallel()
	date := time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC)
	d := fixture()
	r := Evaluate(d, testSpecies, date, d.Latitude, d.Longitude)
	require.NotNil(t, r.Score)
	assert.Equal(t, 50, *r.Score)
	assert.Equal(t, 120, r.Samples)
	assert.Equal(t, 60, r.Reports)
	assert.Equal(t, 3, r.Years)
	assert.Less(t, *r.ScoreLow, *r.Score)
	assert.Greater(t, *r.ScoreHigh, *r.Score)
	assert.Nil(t, Evaluate(nil, testSpecies, date, 0, 0).Score)
	assert.Nil(t, Evaluate(d, "Unmapped species", date, d.Latitude, d.Longitude).Score)
	assert.Nil(t, Evaluate(d, testSpecies, date, 0, 0).Score)
	assert.Nil(t, Evaluate(d, testSpecies, date.AddDate(0, 3, 0), d.Latitude, d.Longitude).Score)
	assert.Nil(t, Evaluate(d, testSpecies, date.AddDate(-2, 0, 0), d.Latitude, d.Longitude).Score)
}

func TestRejectBiasedAndDuplicateSamples(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"incomplete", "duration", "distance", "observers", "protocol", "coordinates", "locality", "shared", "future", "nan"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			d := fixture()
			for i := range d.Checklists {
				c := &d.Checklists[i]
				switch kind {
				case "incomplete":
					c.Complete = false
				case "duration":
					c.DurationMinutes = 181
				case "distance":
					c.DistanceKM = 6
				case "observers":
					c.Observers = 11
				case "protocol":
					c.Protocol = "P20"
				case "coordinates":
					c.Latitude = 91
				case "locality":
					c.LocalityID = "same"
				case "shared":
					c.GroupID = "same"
				case "future":
					c.Date = "2027-05-01"
				case "nan":
					c.DurationMinutes = math.NaN()
				}
			}
			r := Evaluate(d, testSpecies, time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC), d.Latitude, d.Longitude)
			assert.Nil(t, r.Score)
		})
	}
}

func TestSeasonalCalendarBoundaries(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		day, target string
		matches     bool
	}{
		{"2024-12-31", "2026-01-01", true},
		{"2024-12-18", "2026-01-01", true},
		{"2024-12-17", "2026-01-01", false},
		{"2024-02-29", "2026-03-01", true},
		{"2023-05-16", "2026-05-01", false},
		{"2023-02-15", "2026-03-01", true},
		{"2024-02-15", "2026-03-01", false},
		{"2023-02-14", "2024-02-29", true},
		{"2023-02-13", "2024-02-29", false},
	} {
		day, err := wallclock.ParseDate(test.day)
		require.NoError(t, err)
		target, err := wallclock.ParseDate(test.target)
		require.NoError(t, err)
		assert.Equal(t, test.matches, seasonal(day, target), test.day)
	}
}

func TestStoreRefreshAndFailure(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "rarity.json")
	bytes, err := json.Marshal(fixture())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, bytes, 0o600))
	var store Store
	now := time.Now()
	d, err := store.Load(path, now)
	require.NoError(t, err)
	require.NotNil(t, d)
	require.NoError(t, os.WriteFile(path, []byte("invalid"), 0o600))
	cached, err := store.Load(path, now.Add(time.Second))
	require.NoError(t, err)
	assert.Same(t, d, cached)
	refreshed, err := store.Load(path, now.Add(2*time.Minute))
	require.Error(t, err)
	assert.Nil(t, refreshed)
}

func TestScoreCacheSeparatesQueriesAndRefreshes(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data.json")
	d := fixture()
	write := func() {
		t.Helper()
		data, err := json.Marshal(d)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0o600))
	}
	write()
	var store Store
	now := time.Now()
	date := time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC)
	r, err := store.Score(path, now, testSpecies, date, d.Latitude, d.Longitude)
	require.NoError(t, err)
	require.NotNil(t, r.Score)
	assert.Equal(t, 50, *r.Score)
	other, err := store.Score(path, now, "Unknown species", date, d.Latitude, d.Longitude)
	require.NoError(t, err)
	assert.Nil(t, other.Score)
	other, err = store.Score(path, now, testSpecies, date, 0, 0)
	require.NoError(t, err)
	assert.Nil(t, other.Score)
	other, err = store.Score(path, now, testSpecies, date.AddDate(0, 3, 0), d.Latitude, d.Longitude)
	require.NoError(t, err)
	assert.Nil(t, other.Score)
	for i := range d.Checklists {
		d.Checklists[i].Species = nil
	}
	write()
	r, err = store.Score(path, now.Add(2*time.Minute), testSpecies, date, d.Latitude, d.Longitude)
	require.NoError(t, err)
	require.NotNil(t, r.Score)
	assert.GreaterOrEqual(t, *r.Score, 99)
}
