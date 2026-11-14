package species

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/birdnet-go/internal/ebird"
)

func TestCalculateTargetProgress_IntersectionAndDeduplication(t *testing.T) {
	taxonomy := []ebird.TaxonomyEntry{
		{SpeciesCode: "blujay", ScientificName: "Cyanocitta cristata", Category: "species"},
		{SpeciesCode: "gretit1", ScientificName: "Parus major", Category: "species"},
		{SpeciesCode: "hybrid", ScientificName: "Hybrid bird", Category: "hybrid"},
	}
	seen := map[string]struct{}{"Cyanocitta cristata": {}, "Unrelated species": {}, "Hybrid bird": {}}
	result := calculateTargetProgress("US-NJ-001", []string{"blujay", "blujay", "gretit1", "hybrid"}, taxonomy, seen)
	assert.Equal(t, 1, result.Detected)
	assert.Equal(t, 2, result.Total)
	require.NotNil(t, result.Percentage)
	assert.InDelta(t, 50, *result.Percentage, 0.001)
	assert.Equal(t, "US-NJ-001", result.Region)
}

func TestCalculateTargetProgress_EmptyListHasNoInventedPercentage(t *testing.T) {
	result := calculateTargetProgress("US-NJ", nil, nil, nil)
	assert.Zero(t, result.Detected)
	assert.Zero(t, result.Total)
	assert.Nil(t, result.Percentage)
}
