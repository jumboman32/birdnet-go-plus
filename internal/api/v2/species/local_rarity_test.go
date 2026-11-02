package species

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/birdnet-go/internal/api/v2/apitest"
	"github.com/tphakala/birdnet-go/internal/conf"
	"github.com/tphakala/birdnet-go/internal/datastore"
	"github.com/tphakala/birdnet-go/internal/ebird"
	"github.com/tphakala/birdnet-go/internal/rarity"
)

func rarityHandler(t *testing.T, configured bool) *Handler {
	t.Helper()
	return New(apitest.NewCore(t, apitest.WithSettingsFunc(func(s *conf.Settings) {
		s.BirdNET.LocationConfigured = configured
		s.BirdNET.Latitude, s.BirdNET.Longitude = 39.34, -74.57
		s.Realtime.EBird.Enabled = false
	})), nil, nil)
}

func TestGetLocalRarityValidation(t *testing.T) {
	h := rarityHandler(t, true)
	h.RegisterRoutes(h.Group)
	for _, query := range []string{"", "?scientific_name=Cyanocitta+cristata&date=2026-02-30", "?date=2026-05-01"} {
		recorder := httptest.NewRecorder()
		h.Echo.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v2/species/local-rarity"+query, http.NoBody))
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	}
}

func TestGetLocalRarityImportAndMissingData(t *testing.T) {
	t.Setenv("BIRDNET_RARITY_REGION", "US-NJ-001")
	t.Setenv("BIRDNET_LOCAL_RARITY_DATA", "")
	h := rarityHandler(t, true)
	request := func() localRarityResponse {
		t.Helper()
		recorder := httptest.NewRecorder()
		query := url.Values{"scientific_name": {"Cyanocitta cristata"}, "date": {"2024-01-01"}}
		ctx := h.Echo.NewContext(httptest.NewRequest(http.MethodGet, "/?"+query.Encode(), http.NoBody), recorder)
		require.NoError(t, h.GetLocalRarity(ctx))
		require.Equal(t, http.StatusOK, recorder.Code)
		var result localRarityResponse
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
		return result
	}
	r := request()
	assert.Nil(t, r.Score)
	assert.Equal(t, "historical_unavailable", r.RecentStatus)
	assert.Equal(t, "https://dashboard.birdcast.org/region/US-NJ-001?night=2023-12-31", r.BirdcastURL)
	path := filepath.Join(t.TempDir(), "missing-private-path.json")
	t.Setenv("BIRDNET_LOCAL_RARITY_DATA", path)
	r = request()
	assert.Nil(t, r.Score)
	assert.NotContains(t, r.Reason, path)
	d := rarity.Dataset{Version: 1, Source: "Synthetic test import", RetrievedAt: "2026-01-01T00:00:00Z", RegionCode: "US-NJ-001", RegionName: "Atlantic County", Latitude: 39.34, Longitude: -74.57, RadiusKM: 25, Taxa: []string{"Cyanocitta cristata"}}
	bytes, err := json.Marshal(d)
	require.NoError(t, err)
	// A distinct configured path bypasses the intentional failure cache.
	path = filepath.Join(t.TempDir(), "valid.json")
	require.NoError(t, os.WriteFile(path, bytes, 0o600))
	t.Setenv("BIRDNET_LOCAL_RARITY_DATA", path)
	r = request()
	assert.Equal(t, "Atlantic County", r.RegionName)
	assert.Nil(t, r.Score)
	assert.Contains(t, r.Reason, "100 eligible locality-days")
}

func TestBirdcastRegionValidation(t *testing.T) {
	t.Parallel()
	assert.True(t, validBirdcastRegion("US-NJ-001"))
	for _, region := range []string{"", "US-NJ", "US-NJ-00x", "US-NJ-001/../../", "US-NJ-001?url=https://example.com"} {
		assert.False(t, validBirdcastRegion(region))
	}
}

func TestRecentEBirdEvidenceDoesNotCreateScore(t *testing.T) {
	t.Setenv("BIRDNET_LOCAL_RARITY_DATA", "")
	t.Setenv("BIRDNET_RARITY_REGION", "")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, "/data/obs/geo/recent", r.URL.Path)
		assert.Equal(t, "200", r.URL.Query().Get("maxResults"))
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`[{"sciName":"Cyanocitta cristata"}]`))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	client, err := ebird.NewClient(ebird.Config{APIKey: "fixture-key", BaseURL: server.URL, RateLimitMS: 1})
	require.NoError(t, err)
	t.Cleanup(client.Close)
	h := rarityHandler(t, true)
	h.eBirdClientOverride = client
	today := time.Now().In(datastore.Timezone()).Format(time.DateOnly)
	for _, day := range []string{today, "2020-01-01"} {
		recorder := httptest.NewRecorder()
		query := url.Values{"scientific_name": {"Cyanocitta cristata"}, "date": {day}}
		ctx := h.Echo.NewContext(httptest.NewRequest(http.MethodGet, "/?"+query.Encode(), http.NoBody), recorder)
		require.NoError(t, h.GetLocalRarity(ctx))
		var result localRarityResponse
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
		assert.Nil(t, result.Score)
		if day == today {
			assert.Equal(t, "available", result.RecentStatus)
			assert.True(t, result.RecentReported)
		} else {
			assert.Equal(t, "historical_unavailable", result.RecentStatus)
			assert.False(t, result.RecentReported)
		}
	}
	assert.EqualValues(t, 1, calls.Load(), "historical dates must not fetch today's observations")
}
