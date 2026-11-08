package classifier

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/birdnet-go/internal/conf"
	"github.com/tphakala/birdnet-go/internal/conf/conftest"
)

func localProfileFixture(t *testing.T) (*conf.Settings, *localOccurrenceProfile) {
	t.Helper()
	settings := conftest.GetTestSettings()
	settings.BirdNET.Latitude = 39.5
	settings.BirdNET.Longitude = -74.5
	settings.BirdNET.LocationConfigured = true
	settings.BirdNET.RangeFilter.Threshold = 0.01
	settings.BirdNET.RangeFilter.LocalDataPath = filepath.Join(t.TempDir(), "test.ebird.local.json")
	profile := &localOccurrenceProfile{
		Version: localOccurrenceVersion, Region: "US-NJ-001",
		Bounds: localOccurrenceBounds{South: 39, North: 40, West: -75, East: -74},
	}
	for month := 1; month <= localOccurrenceMonths; month++ {
		profile.Months = append(profile.Months, localOccurrenceMonth{
			Month: month, Checklists: 1000, Species: map[string]int{"Corvus corax": 50},
		})
	}
	writeLocalProfileFixture(t, settings.BirdNET.RangeFilter.LocalDataPath, profile)
	return settings, profile
}

func writeLocalProfileFixture(t *testing.T, path string, profile *localOccurrenceProfile) {
	t.Helper()
	data, err := json.Marshal(profile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func TestLocalOccurrence_SeasonLocationEvidenceAndExclusions(t *testing.T) {
	for _, name := range []string{"admitted", "excluded", "wrong month", "outside region", "no location", "few reports", "sparse month", "below threshold", "disabled", "unsupported"} {
		t.Run(name, func(t *testing.T) {
			settings, profile := localProfileFixture(t)
			labels := []string{"Corvus corax_Common Raven"}
			date := time.Date(2026, time.November, 7, 12, 0, 0, 0, time.UTC)
			switch name {
			case "excluded":
				settings.Realtime.Species.Exclude = []string{"Corvus corax"}
			case "wrong month":
				delete(profile.Months[10].Species, "Corvus corax")
			case "outside region":
				settings.BirdNET.Latitude = 60
			case "no location":
				settings.BirdNET.LocationConfigured = false
			case "few reports":
				profile.Months[10].Species["Corvus corax"] = 4
			case "sparse month":
				profile.Months[10].Checklists = 99
			case "below threshold":
				settings.BirdNET.RangeFilter.Threshold = 0.1
			case "disabled":
				settings.BirdNET.RangeFilter.LocalDataPath = ""
			case "unsupported":
				labels = []string{"Turdus merula_Common Blackbird"}
			case "admitted":
				// The fixture satisfies all admission checks.
			}
			if name != "disabled" {
				writeLocalProfileFixture(t, settings.BirdNET.RangeFilter.LocalDataPath, profile)
			}
			rfs := newRangeFilterService(nil)
			got := rfs.supplementLocalSpecies(nil, date, settings, &rangeFilterState{coveredLabels: labels},
				newExcludeMatcher(settings.Realtime.Species.Exclude, settings.BirdNET.Locale))
			if name == "admitted" {
				require.Len(t, got, 1)
				assert.InDelta(t, 0.05, got[0].Score, 0.000001)
				assert.True(t, got[0].IsSyntheticOverride)
				assert.Empty(t, buildOccurrenceIndex(got), "county checklist frequency must not become geomodel rarity")
			} else {
				assert.Empty(t, got)
			}
		})
	}
}

func TestLocalOccurrence_PreservesNativeScoresAndCanonicalDedup(t *testing.T) {
	settings, _ := localProfileFixture(t)
	rfs := newRangeFilterService(nil)
	existing := []SpeciesScore{{Label: "Corvus corax_Northern Raven", Score: 0.9}}
	state := &rangeFilterState{coveredLabels: []string{"Corvus corax_Common Raven"}, participants: []participantLabels{{labels: []string{"Corvus corax"}}}}
	got := rfs.supplementLocalSpecies(existing, time.Now(), settings, state, newExcludeMatcher(nil, settings.BirdNET.Locale))
	assert.Equal(t, existing, got)
}

func TestLocalOccurrence_ProfileReplacementRemovalAndConcurrentReads(t *testing.T) {
	settings, profile := localProfileFixture(t)
	path := settings.BirdNET.RangeFilter.LocalDataPath
	cache := &localOccurrenceCache{}
	require.NotNil(t, cache.load(path))
	profile.Months[0].Species["Corvus corax"] = 123
	writeLocalProfileFixture(t, path, profile)
	updated := time.Now().Add(time.Second)
	require.NoError(t, os.Chtimes(path, updated, updated))
	var workers sync.WaitGroup
	for range 10 {
		workers.Go(func() {
			got := cache.load(path)
			assert.Equal(t, 123, got.Months[0].Species["Corvus corax"])
		})
	}
	workers.Wait()
	require.NoError(t, os.Remove(path))
	assert.Nil(t, cache.load(path), "do not retain stale evidence after removal")
	writeLocalProfileFixture(t, path, profile)
	assert.NotNil(t, cache.load(path), "recover when the profile is restored")
	require.NoError(t, os.WriteFile(path, []byte("broken"), 0o600))
	assert.Nil(t, cache.load(path), "a malformed replacement must disable only the supplement")
}

func TestLocalOccurrence_InvalidProfiles(t *testing.T) {
	_, profile := localProfileFixture(t)
	valid, err := json.Marshal(profile)
	require.NoError(t, err)
	for _, data := range []string{
		"broken", "{}", string(valid) + "{}",
		strings.Replace(string(valid), `"version":1`, `"version":2`, 1),
		strings.Replace(string(valid), `"checklists":1000`, `"checklists":1`, 1),
		strings.Replace(string(valid), `"south":39`, `"south":91`, 1),
		strings.Repeat(" ", localOccurrenceMaxBytes+1),
	} {
		_, err := decodeLocalOccurrence(strings.NewReader(data))
		require.Error(t, err)
	}
}

// This covers startup, the preview and the method used by the daily rebuild,
// using both native backend paths rather than testing only the profile helper.
func TestLocalOccurrence_GatePreviewAndDailyUpdateAgree(t *testing.T) {
	for _, universal := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "universal"}[universal], func(t *testing.T) {
			settings, _ := localProfileFixture(t)
			labels := []string{"Turdus merula_Common Blackbird", "Corvus corax_Common Raven"}
			settings.BirdNET.Labels = labels
			conftest.SetTestSettings(settings)
			t.Cleanup(func() { conftest.SetTestSettings(nil) })
			var o *Orchestrator
			if universal {
				rf := &fakeUniversalRangeFilter{geoLabels: labels,
					scores: []SpeciesScore{{Label: labels[0], Score: 0.9}}, rawScores: []float32{0.9, 0}}
				o = buildV24PlusPerchOrchestrator(t, settings, rf, rfKindGeomodelV3, labels, nil)
			} else {
				o = buildV24PlusPerchOrchestrator(t, settings, &fakeRangeFilter{scores: []float32{0.9, 0}}, rfKindMDataV2, labels, nil)
			}
			require.NoError(t, BuildRangeFilter(o))
			assert.True(t, conf.GetSettings().IsSpeciesIncluded(labels[1]))
			gate := canonicalSet(conf.GetSettings().GetIncludedSpecies())
			preview, err := o.GetAllProbableSpeciesWithSettings(time.Now(), 0, settings)
			require.NoError(t, err)
			assert.Equal(t, gate, canonicalSet(speciesScoreLabels(preview)))
			daily, err := o.GetProbableSpecies(time.Now(), 0)
			require.NoError(t, err)
			assert.Equal(t, gate, canonicalSet(speciesScoreLabels(daily)))
		})
	}
}
