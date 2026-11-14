package visual

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/birdnet-go/internal/api/v2/apitest"
	"github.com/tphakala/birdnet-go/internal/conf"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/entities"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/repository"
	"github.com/tphakala/birdnet-go/internal/speciesindex"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const cardinalScientificName = "Cardinalis cardinalis" //nolint:misspell // Cardinalis is the scientific genus, not the English plural.

func visualTestHandler(t *testing.T) *Handler {
	t.Helper()
	core := apitest.NewCore(t, apitest.WithoutSettingsPublish(), apitest.WithSettingsFunc(func(s *conf.Settings) { s.Realtime.Frigate.CorrelationWindow = 30 }))
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "visual.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&entities.VisualDetection{}, &entities.VisualSyncState{}, &entities.LabelType{}, &entities.TaxonomicClass{}, &entities.AIModel{}, &entities.Label{}, &entities.AudioSource{}, &entities.Detection{}))
	require.NoError(t, db.Create(&entities.LabelType{ID: 1, Name: entities.LabelTypeSpecies}).Error)
	require.NoError(t, db.Create(&entities.AIModel{ID: 1, Name: "test", Version: "1", Variant: "default", ModelType: entities.ModelTypeBird}).Error)
	h := New(core, nil)
	h.repo = repository.NewVisualRepository(db, "")
	h.audio = repository.NewDetectionRepository(db, nil, false, false)
	h.labels = repository.NewLabelRepository(db, nil, false, false)
	return h
}

func TestFrigateRoutesRequireAuth(t *testing.T) {
	t.Parallel()
	h := visualTestHandler(t)
	h.AuthMiddleware = func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error { return echo.NewHTTPError(http.StatusUnauthorized) }
	}
	h.RegisterRoutes(h.Group)
	for _, path := range []string{"/visual-detections", "/visual-detections/summary", "/visual-detections/1", "/visual-detections/1/snapshot", "/visual-detections/1/correlations", "/visual-detections/1/species", "/integrations/frigate/status", "/integrations/frigate/test", "/integrations/frigate/cameras", "/integrations/frigate/cameras/feeder/live", "/integrations/frigate/cameras/feeder/source"} {
		method := http.MethodGet
		if path == "/integrations/frigate/test" {
			method = http.MethodPost
		}
		if path == "/visual-detections/1/species" {
			method = http.MethodPut
		}
		response := httptest.NewRecorder()
		h.Echo.ServeHTTP(response, httptest.NewRequest(method, "/api/v2"+path, http.NoBody))
		assert.Equal(t, http.StatusUnauthorized, response.Code, path)
	}
}

func TestFrigateOwnerSpeciesReview(t *testing.T) {
	t.Parallel()
	h := visualTestHandler(t)
	names := speciesindex.Build([]string{cardinalScientificName + "_Northern Cardinal"}, nil, "en")
	h.names = func() *speciesindex.Snapshot { return names }
	require.NoError(t, h.repo.Upsert(t.Context(), []entities.VisualDetection{{Source: "frigate", Instance: "test", ExternalEventID: "review-bird", Camera: "feeder", ObjectLabel: "bird", StartTime: time.Now().UTC()}}))
	rows, _, err := h.repo.List(t.Context(), &repository.VisualFilter{})
	require.NoError(t, err)
	invoke := func(body string) error {
		request := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
		request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		ctx := h.Echo.NewContext(request, httptest.NewRecorder())
		ctx.SetParamNames("id")
		ctx.SetParamValues("1")
		return h.reviewSpecies(ctx)
	}
	require.Error(t, invoke(`{"scientificName":"not a real indexed species"}`))
	require.NoError(t, invoke(`{"scientificName":"`+cardinalScientificName+`"}`))
	row, err := h.repo.Get(t.Context(), rows[0].ID)
	require.NoError(t, err)
	assert.True(t, row.SpeciesVerified)
	assert.Equal(t, entities.VisualClassificationManual, row.ClassificationSource)
	assert.Equal(t, new(cardinalScientificName), row.ScientificName)
	assert.Nil(t, row.ClassificationConfidence)
	require.NoError(t, invoke(`{"verified":false}`))
	row, err = h.repo.Get(t.Context(), rows[0].ID)
	require.NoError(t, err)
	assert.False(t, row.SpeciesVerified)
	assert.Nil(t, row.ScientificName)
}

func TestFrigateVisualFilters(t *testing.T) {
	t.Parallel()
	h := visualTestHandler(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	rows := []entities.VisualDetection{
		{Source: "frigate", Instance: "one", ExternalEventID: "identified", Camera: "feeder", ObjectLabel: "bird", ScientificName: new(cardinalScientificName), ClassificationConfidence: new(0.9), StartTime: now, SnapshotStatus: "available"},
		{Source: "frigate", Instance: "one", ExternalEventID: "unknown", Camera: "other", ObjectLabel: "bird", StartTime: now, SnapshotStatus: "missing"},
	}
	require.NoError(t, h.repo.Upsert(t.Context(), rows))
	for _, query := range []string{"camera=feeder", "identified=true", "snapshot=true", "minConfidence=0.8", "q=cardinal", "q=FEEDER", "species=" + url.QueryEscape(cardinalScientificName)} {
		response := httptest.NewRecorder()
		ctx := h.Echo.NewContext(httptest.NewRequest(http.MethodGet, "/visual-detections?"+query, http.NoBody), response)
		require.NoError(t, h.list(ctx))
		var result struct {
			Total int64 `json:"total"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		assert.EqualValues(t, 1, result.Total)
	}
	for _, query := range []string{"q=%25", "q=_", "q=not-a-species"} {
		response := httptest.NewRecorder()
		ctx := h.Echo.NewContext(httptest.NewRequest(http.MethodGet, "/visual-detections?"+query, http.NoBody), response)
		require.NoError(t, h.list(ctx))
		var result struct {
			Total int64 `json:"total"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		assert.Zero(t, result.Total)
	}
	for _, query := range []string{"limit=0", "offset=-1", "minConfidence=NaN", "identified=maybe", "start=invalid"} {
		ctx := h.Echo.NewContext(httptest.NewRequest(http.MethodGet, "/?"+query, http.NoBody), httptest.NewRecorder())
		_, err := parseFilter(ctx)
		require.Error(t, err)
	}
}

func TestFrigateCorrelationPreservesConfidence(t *testing.T) {
	t.Parallel()
	h := visualTestHandler(t)
	const observedUnix = 1700000000
	now := time.Unix(observedUnix, int64(time.Second/2)).UTC()
	row := entities.VisualDetection{Source: "frigate", Instance: "one", ExternalEventID: "correlate", Camera: "feeder", ObjectLabel: "bird", ScientificName: new(cardinalScientificName), ClassificationConfidence: new(0.9), StartTime: now, SnapshotStatus: "missing"}
	require.NoError(t, h.repo.Upsert(t.Context(), []entities.VisualDetection{row}))
	stored, _, err := h.repo.List(t.Context(), &repository.VisualFilter{})
	require.NoError(t, err)
	require.Len(t, stored, 1)
	label, err := h.labels.GetOrCreate(t.Context(), cardinalScientificName, 1, 1, nil)
	require.NoError(t, err)
	other, err := h.labels.GetOrCreate(t.Context(), "Parus major", 1, 1, nil)
	require.NoError(t, err)
	for _, fixture := range []struct {
		label  uint
		offset int64
	}{
		{label.ID, 5}, {label.ID, 30}, // inside, including the fractional upper second
		{label.ID, -30}, {label.ID, 31}, {label.ID, 3600}, // outside the inclusive 30-second window
		{other.ID, 5},
	} {
		require.NoError(t, h.audio.Save(t.Context(), &entities.Detection{LabelID: fixture.label, ModelID: 1, DetectedAt: observedUnix + fixture.offset, Confidence: 0.7}))
	}
	response := httptest.NewRecorder()
	ctx := h.Echo.NewContext(httptest.NewRequest(http.MethodGet, "/", http.NoBody), response)
	ctx.SetParamNames("id")
	ctx.SetParamValues("1")
	require.NoError(t, h.correlations(ctx))
	assert.Contains(t, response.Body.String(), `"relationship":"potential"`)
	assert.Contains(t, response.Body.String(), `"audioConfidence":0.7`)
	var result struct {
		Matches []potentialMatch `json:"matches"`
		Total   int64            `json:"total"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	assert.EqualValues(t, 2, result.Total)
	require.Len(t, result.Matches, 2)
	for _, match := range result.Matches {
		assert.LessOrEqual(t, match.DeltaSeconds, 30.0)
		assert.GreaterOrEqual(t, match.DeltaSeconds, -30.0)
		assert.Equal(t, cardinalScientificName, match.ScientificName)
	}
	after, err := h.repo.Get(t.Context(), stored[0].ID)
	require.NoError(t, err)
	assert.Equal(t, new(0.9), after.ClassificationConfidence)
}
