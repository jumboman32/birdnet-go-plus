package frigate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/birdnet-go/internal/conf"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/entities"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/repository"
	"github.com/tphakala/birdnet-go/internal/speciesindex"
)

func TestFrigateDelayedSublabelAfterCursorAndManualReview(t *testing.T) {
	t.Parallel()
	for _, prefix := range []string{"", "v2_"} {
		t.Run(prefix, func(t *testing.T) {
			repo := testRepository(t, prefix)
			observed := time.Now().Add(-10 * time.Minute).UTC().Truncate(time.Microsecond)
			ended := observed.Add(time.Minute)
			var phase atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				after, err := strconv.ParseFloat(r.URL.Query().Get("after"), 64)
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				before, err := strconv.ParseFloat(r.URL.Query().Get("before"), 64)
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				start := float64(observed.UnixMicro()) / float64(time.Second/time.Microsecond)
				if after > start || before <= start {
					_, _ = w.Write([]byte("[]"))
					return
				}
				event := Event{ID: "delayed-bird", Camera: "feeder", Label: "bird", Start: float64(observed.UnixMicro()) / float64(time.Second/time.Microsecond), End: new(float64(ended.UnixMicro()) / float64(time.Second/time.Microsecond)), HasSnapshot: true, SubLabel: json.RawMessage(`null`)}
				switch phase.Load() {
				case 1:
					event.SubLabel = json.RawMessage(`["Northern Cardinal",0.94]`)
				case 2:
					event.SubLabel = json.RawMessage(`["Blue Jay",0.89]`)
				}
				_ = json.NewEncoder(w).Encode([]Event{event})
			}))
			defer server.Close()
			cfg := config(server.URL)
			cfg.InitialHistoryHours = 1
			names := speciesindex.Build([]string{cardinalScientificName + "_Northern Cardinal", "Cyanocitta cristata_Blue Jay"}, nil, "en")
			service := NewService(repo, func() conf.FrigateSettings { return cfg }, func() *speciesindex.Snapshot { return names }, nil, nil)
			require.NoError(t, service.Sync(t.Context(), &cfg))
			rows, total, err := repo.List(t.Context(), &repository.VisualFilter{})
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			id := rows[0].ID
			assert.Nil(t, rows[0].ScientificName)
			cursor, err := repo.Cursor(t.Context(), cursorKey(&cfg))
			require.NoError(t, err)
			assert.True(t, cursor.After(observed))
			phase.Store(1)
			require.NoError(t, service.Sync(t.Context(), &cfg))
			row, err := repo.Get(t.Context(), id)
			require.NoError(t, err)
			assert.Equal(t, new(cardinalScientificName), row.ScientificName)
			assert.Equal(t, new(0.94), row.ClassificationConfidence)
			assert.Equal(t, entities.VisualClassificationFrigate, row.ClassificationSource)
			phase.Store(2)
			require.NoError(t, service.Sync(t.Context(), &cfg))
			row, err = repo.Get(t.Context(), id)
			require.NoError(t, err)
			assert.Equal(t, new("Cyanocitta cristata"), row.ScientificName)
			require.NoError(t, repo.ReviewSpecies(t.Context(), id, cardinalScientificName, "Northern Cardinal", true))
			require.NoError(t, service.Sync(t.Context(), &cfg))
			row, err = repo.Get(t.Context(), id)
			require.NoError(t, err)
			assert.True(t, row.SpeciesVerified)
			assert.Equal(t, new(cardinalScientificName), row.ScientificName)
			assert.Equal(t, entities.VisualClassificationManual, row.ClassificationSource)
			assert.Nil(t, row.ClassificationConfidence)
			assert.WithinDuration(t, observed, row.StartTime, time.Microsecond)
			require.NotNil(t, row.EndTime)
			assert.WithinDuration(t, ended, *row.EndTime, time.Microsecond)
			assert.Equal(t, SnapshotAvailable, row.SnapshotStatus)
			require.NoError(t, repo.ReviewSpecies(t.Context(), id, "", "", false))
			// A fresh service uses the durable cursor and keeps event IDs idempotent.
			service = NewService(repo, func() conf.FrigateSettings { return cfg }, func() *speciesindex.Snapshot { return names }, nil, nil)
			require.NoError(t, service.Sync(t.Context(), &cfg))
			row, err = repo.Get(t.Context(), id)
			require.NoError(t, err)
			assert.Equal(t, new("Cyanocitta cristata"), row.ScientificName)
			_, total, err = repo.List(t.Context(), &repository.VisualFilter{})
			require.NoError(t, err)
			assert.EqualValues(t, 1, total)
		})
	}
}

func TestFrigateLowConfidenceRemainsUnidentified(t *testing.T) {
	t.Parallel()
	names := speciesindex.Build([]string{cardinalScientificName + "_Northern Cardinal"}, nil, "en")
	service := NewService(nil, nil, func() *speciesindex.Snapshot { return names }, nil, nil)
	const threshold = 0.7
	event := Event{ID: "low-confidence-bird", Camera: "feeder", Label: "bird", Start: 1700000000, SubLabel: json.RawMessage(`["Northern Cardinal",0.4]`)}
	row, err := event.Observation("test")
	require.NoError(t, err)
	service.normalize(row, threshold)
	assert.Nil(t, row.ScientificName)
	assert.Nil(t, row.CommonName)
	assert.Equal(t, new(0.4), row.ClassificationConfidence)
	assert.Nil(t, row.DetectionConfidence)
	assert.Equal(t, entities.VisualClassificationFrigate, row.ClassificationSource)
}
