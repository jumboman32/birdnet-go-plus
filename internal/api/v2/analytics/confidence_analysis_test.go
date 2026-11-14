package analytics

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfidenceAnalysis_ValidationAndUnavailable(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		status      int
	}{
		{"missing date", "format=analysis", http.StatusBadRequest},
		{"invalid date", "format=analysis&start_date=2026-02-30", http.StatusBadRequest},
		{"reversed dates", "format=analysis&start_date=2026-03-02&end_date=2026-03-01", http.StatusBadRequest},
		{"no enhanced database", "format=analysis&start_date=2026-03-01&end_date=2026-03-02", http.StatusNotImplemented},
		{"no enhanced review", "format=review&start_date=2026-03-01&end_date=2026-03-02&model=1&bin=4", http.StatusNotImplemented},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, _, handler := setupAnalyticsTestEnvironment(t)
			ctx, recorder := newAnalyticsContext(e, "/api/v2/analytics/confidence/distribution?"+tc.query)
			err := handler.GetConfidenceDistribution(ctx)
			if err != nil {
				require.ErrorIs(t, err, ErrResponseHandled)
			}
			assert.Equal(t, tc.status, recorder.Code)
		})
	}
}
