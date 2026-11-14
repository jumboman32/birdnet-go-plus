package repository

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfidenceAnalysis_FullPopulationAndBoundaries(t *testing.T) {
	db := setupInsightsTestDBMultiModel(t)
	repo := NewConfidenceAnalyticsRepository(db, false)
	label := seedLabel(t, db, "Parus major")
	other := seedLabelForModel(t, db, "Parus major", 2)
	for _, score := range []float64{0, 0.2, 0.25, 0.3, 0.79, 0.8, 1} {
		seedDetection(t, db, label, 1000, score)
	}
	seedDetectionForModel(t, db, other, 2, 1000, 0.9)
	seedFalsePositiveReview(t, db, seedDetection(t, db, label, 1000, 0.99))
	seedDetection(t, db, label, 2000, 0.5) // End is exclusive.
	seedDetection(t, db, label, 999, 0.5)
	seedDetection(t, db, label, 1000, 1.1) // Invalid historical score.
	rows, err := repo.Distribution(t.Context(), 1000, 2000, nil, nil)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, int64(7), rows[0].Count)
	assert.Equal(t, int64(2), rows[0].Low)
	assert.Equal(t, int64(2), rows[0].Middle)
	assert.Equal(t, int64(2), rows[0].High)
	assert.Equal(t, int64(1), rows[0].Bins[19])
	assert.InDelta(t, 0.3, rows[0].Median, confidenceTolerance)
	assert.InDelta(t, (0.2+0.25+0.3+0.79+0.8+1)/7, rows[0].Average, confidenceTolerance)
	assert.Equal(t, int64(1), rows[1].Count, "one-detection series must not be hidden")
	assert.InDelta(t, 0.9, rows[1].Median, confidenceTolerance)
	assert.Equal(t, "Parus major", rows[0].TopSpecies)
	assert.Equal(t, 1, rows[0].UniqueSpecies)
	review, err := repo.Review(t.Context(), 1000, 2000, "", 1, 5, 0)
	require.NoError(t, err)
	require.Len(t, review, 1, "25–30% excludes the exact 30% boundary")
	assert.InDelta(t, 0.25, review[0].Confidence, confidenceTolerance)
}

func TestConfidenceAnalysis_FilteringAndEvenMedian(t *testing.T) {
	db := setupInsightsTestDBMultiModel(t)
	repo := NewConfidenceAnalyticsRepository(db, false)
	parus := seedLabel(t, db, "Parus major")
	turdus := seedLabel(t, db, "Turdus merula")
	seedDetection(t, db, parus, 1000, 0.2)
	seedDetection(t, db, parus, 1000, 0.8)
	seedDetection(t, db, turdus, 1000, 1)
	rows, err := repo.Distribution(t.Context(), 0, 2000, []string{"Parus major"}, []uint{1})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "Parus major", rows[0].Species)
	assert.Equal(t, int64(2), rows[0].Count)
	assert.InDelta(t, 0.5, rows[0].Median, confidenceTolerance)
	empty, err := repo.Distribution(t.Context(), 0, 2000, []string{"Missing species"}, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
	assert.NotNil(t, empty)
}

func TestConfidenceAnalysis_ReviewIsBoundedAndCancellable(t *testing.T) {
	db := setupInsightsTestDB(t)
	repo := NewConfidenceAnalyticsRepository(db, false)
	label := seedLabel(t, db, "Parus major")
	const detections = 30
	for i := range detections {
		seedDetection(t, db, label, int64(1000+i), 1)
	}
	first, err := repo.Review(t.Context(), 0, 2000, "", 1, 19, 0)
	require.NoError(t, err)
	assert.Len(t, first, 25)
	second, err := repo.Review(t.Context(), 0, 2000, "", 1, 19, 25)
	require.NoError(t, err)
	assert.Len(t, second, 5)
	assert.Greater(t, first[len(first)-1].DetectedAt, second[0].DetectedAt)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = repo.Distribution(ctx, 0, 2000, nil, nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestConfidenceAnalysis_FloatingBoundaryMatchesReview(t *testing.T) {
	db := setupInsightsTestDB(t)
	repo := NewConfidenceAnalyticsRepository(db, false)
	label := seedLabel(t, db, "Parus major")
	for _, score := range []float64{math.Nextafter(0.45, 0), 0.45, math.Nextafter(0.9, 0), 0.9} {
		seedDetection(t, db, label, 1000, score)
	}
	rows, err := repo.Distribution(t.Context(), 0, 2000, nil, nil)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	for _, bin := range []int{8, 9, 17, 18} {
		review, err := repo.Review(t.Context(), 0, 2000, "", 1, bin, 0)
		require.NoError(t, err)
		assert.Len(t, review, 1)
		assert.Equal(t, int64(len(review)), rows[0].Bins[bin])
	}
}
