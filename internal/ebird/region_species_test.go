package ebird

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRegionSpecies_CachesAndReturnsCopies(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		calls.Add(1)
		assert.Equal(t, "/product/spplist/US-NJ-001", r.URL.Path)
		assert.NotEmpty(t, r.Header.Get("X-eBirdApiToken"))
		_, err := w.Write([]byte(`["blujay","norcar"]`))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	client := setupTestClient(t, server)
	first, err := client.GetRegionSpecies(t.Context(), "US-NJ-001")
	require.NoError(t, err)
	require.Len(t, first, 2)
	first[0] = "changed"
	second, err := client.GetRegionSpecies(t.Context(), "US-NJ-001")
	require.NoError(t, err)
	assert.Equal(t, []string{"blujay", "norcar"}, second)
	assert.Equal(t, int32(1), calls.Load())
	_, err = client.GetRegionSpecies(t.Context(), "US-NJ-001/../secret")
	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load(), "invalid regions must not make requests")
}
