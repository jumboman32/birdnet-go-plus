package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/entities"
)

func TestFrigateSQLTimePreservesUTC(t *testing.T) {
	t.Parallel()
	const offsetHours = -5
	local := time.FixedZone("station", offsetHours*int(time.Hour/time.Second))
	instant := time.Date(2026, time.November, 13, 12, 30, 0, 0, local)
	encoded := visualSQLTime(instant)
	assert.Equal(t, instant.UnixMicro(), encoded)
	assert.Equal(t, "2026-11-13T17:30:00Z", time.UnixMicro(encoded).UTC().Format(time.RFC3339))
	row := entities.VisualDetection{StartUnixMicro: encoded, EndUnixMicro: &encoded, CreatedUnixMicro: encoded, UpdatedUnixMicro: encoded}
	(&visualRepository{}).restoreUTC(&row)
	assert.True(t, row.StartTime.Equal(instant))
	assert.Equal(t, time.UTC, row.StartTime.Location())
	assert.True(t, row.EndTime.Equal(instant))
	values := visualValues(&entities.VisualDetection{StartTime: instant, EndTime: &instant})
	assert.Equal(t, encoded, values["start_time"])
	assert.Equal(t, encoded, values["end_time"])
	assert.Nil(t, visualSQLTimePointer(nil))
}
