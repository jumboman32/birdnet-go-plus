package frigate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	"github.com/tphakala/birdnet-go/internal/testutil"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

const cardinalScientificName = "Cardinalis cardinalis" //nolint:misspell // Cardinalis is the scientific genus, not the English plural.

func config(base string) conf.FrigateSettings {
	return conf.FrigateSettings{Enabled: true, BaseURL: base, Cameras: []string{"feeder"}, PollInterval: 20, RequestTimeout: 1, MaxAttempts: 1, InitialHistoryHours: 24, OverlapSeconds: 3600, CorrelationWindow: 30, SnapshotRetentionHours: 168, SnapshotImport: true}
}

func testRepository(t *testing.T, prefix string) repository.VisualRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "visual.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), NamingStrategy: schema.NamingStrategy{TablePrefix: prefix}})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&entities.VisualDetection{}, &entities.VisualSyncState{}))
	return repository.NewVisualRepository(db, prefix)
}

func TestFrigateParsing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, raw, label string
		confidence       *float64
	}{
		{"identified", `"Northern Cardinal"`, "Northern Cardinal", nil},
		{"tuple", `["Northern Cardinal",0.9]`, "Northern Cardinal", new(0.9)},
		{"unknown", `null`, "", nil},
		{"malformed", `{"unexpected":true}`, "", nil},
		{"empty", `[]`, "", nil},
		{"invalid score", `["Northern Cardinal",2]`, "Northern Cardinal", nil},
		{"bad score type", `["Northern Cardinal","bad"]`, "Northern Cardinal", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Event{ID: "123-bird", Camera: "feeder", Label: "bird", Start: 1700000000.25, SubLabel: json.RawMessage(tc.raw)}
			row, err := e.Observation("instance")
			require.NoError(t, err)
			assert.Equal(t, time.UTC, row.StartTime.Location())
			assert.Equal(t, int64(1700000000250), row.StartTime.UnixMilli())
			assert.Equal(t, tc.confidence, row.ClassificationConfidence)
			if tc.label == "" {
				assert.Nil(t, row.OriginalLabel)
			} else {
				require.NotNil(t, row.OriginalLabel)
				assert.Equal(t, tc.label, *row.OriginalLabel)
			}
			assert.Nil(t, row.ScientificName)
			assert.Nil(t, row.DetectionConfidence)
		})
	}
}

func TestFrigateImportUpdateAndRecovery(t *testing.T) {
	t.Parallel()
	for _, prefix := range []string{"", "v2_"} {
		t.Run(prefix, func(t *testing.T) {
			repo := testRepository(t, prefix)
			var classified, offline atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if offline.Load() {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				q := r.URL.Query()
				if q.Get("labels") != "bird" || q.Get("cameras") != "feeder" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				label := "null"
				if classified.Load() {
					label = `["Northern Cardinal",0.94]`
				}
				_, _ = fmt.Fprintf(w, `[{"id":"123-bird","camera":"feeder","label":"bird","start_time":%f,"sub_label":%s,"has_snapshot":true,"data":{"top_score":0.8}}]`, float64(time.Now().Add(-time.Minute).Unix()), label)
			}))
			defer server.Close()
			cfg := config(server.URL)
			names := speciesindex.Build([]string{cardinalScientificName + "_Northern Cardinal"}, nil, "en")
			svc := NewService(repo, func() conf.FrigateSettings { return cfg }, func() *speciesindex.Snapshot { return names }, nil, nil)
			require.NoError(t, svc.Sync(t.Context(), &cfg))
			rows, total, err := repo.List(t.Context(), &repository.VisualFilter{})
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Len(t, rows, 1)
			assert.Nil(t, rows[0].ScientificName)
			id := rows[0].ID
			classified.Store(true)
			require.NoError(t, svc.Sync(t.Context(), &cfg))
			require.NoError(t, svc.Sync(t.Context(), &cfg))
			row, err := repo.Get(t.Context(), id)
			require.NoError(t, err)
			assert.Equal(t, new(cardinalScientificName), row.ScientificName)
			assert.Equal(t, new(0.94), row.ClassificationConfidence)
			cursor, err := repo.Cursor(t.Context(), cursorKey(&cfg))
			require.NoError(t, err)
			assert.False(t, cursor.IsZero())
			offline.Store(true)
			require.Error(t, svc.Sync(t.Context(), &cfg))
			retained, err := repo.Cursor(t.Context(), cursorKey(&cfg))
			require.NoError(t, err)
			assert.True(t, cursor.Equal(retained))
			offline.Store(false)
			require.NoError(t, svc.Sync(t.Context(), &cfg))
			_, total, err = repo.List(t.Context(), &repository.VisualFilter{})
			require.NoError(t, err)
			assert.EqualValues(t, 1, total)
			summary, err := repo.Summary(t.Context(), time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
			require.NoError(t, err)
			assert.EqualValues(t, 1, summary.Total)
			assert.EqualValues(t, 1, summary.UniqueSpecies)
			assert.Zero(t, summary.Unidentified)
			cfg.Enabled = false
			require.NoError(t, svc.Sync(t.Context(), &cfg))
		})
	}
}

func TestFrigatePaginationAndBackfill(t *testing.T) {
	t.Parallel()
	repo := testRepository(t, "")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		count := pageSize
		if offset >= pageSize {
			count = 1
		}
		events := make([]Event, count)
		for i := range events {
			events[i] = Event{ID: fmt.Sprintf("event-%d", offset+i), Camera: "feeder", Label: "bird", Start: float64(time.Now().Add(-2 * time.Hour).Unix())}
		}
		_ = json.NewEncoder(w).Encode(events)
	}))
	defer server.Close()
	cfg := config(server.URL)
	svc := NewService(repo, func() conf.FrigateSettings { return cfg }, nil, nil, nil)
	require.NoError(t, svc.Sync(t.Context(), &cfg))
	assert.EqualValues(t, 2, requests.Load())
	_, total, err := repo.List(t.Context(), &repository.VisualFilter{})
	require.NoError(t, err)
	assert.EqualValues(t, pageSize+1, total)
	// A fresh service resumes the same durable cursor and safely replays duplicates.
	restarted := NewService(repo, func() conf.FrigateSettings { return cfg }, nil, nil, nil)
	require.NoError(t, restarted.Sync(t.Context(), &cfg))
	_, total, err = repo.List(t.Context(), &repository.VisualFilter{})
	require.NoError(t, err)
	assert.EqualValues(t, pageSize+1, total)
}

func TestFrigateHTTPFailures(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusFound, http.StatusServiceUnavailable} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "http://should-not-follow.invalid")
				w.WriteHeader(status)
				_, _ = w.Write([]byte("private-secret"))
			}))
			defer server.Close()
			client, err := NewClient(new(config(server.URL)), nil)
			require.NoError(t, err)
			err = client.TestConnection(t.Context())
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "private-secret")
			assert.NotContains(t, err.Error(), server.URL)
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	client, err := NewClient(new(config(server.URL)), nil)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, client.TestConnection(ctx), context.Canceled)
	// The configured HTTP timeout also bounds requests without caller cancellation.
	require.ErrorContains(t, client.TestConnection(t.Context()), "timed out")
}

func TestFrigateSnapshots(t *testing.T) {
	t.Parallel()
	var b bytes.Buffer
	require.NoError(t, jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil))
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(b.Bytes())
	}))
	defer server.Close()
	cfg := config(server.URL)
	client, err := NewClient(&cfg, nil)
	require.NoError(t, err)
	row := &entities.VisualDetection{Instance: InstanceKey(server.URL), ExternalEventID: "123-bird", EndTime: new(time.Now().UTC())}
	cache := NewSnapshotCache()
	directory := filepath.Join(t.TempDir(), "snapshots")
	first, err := cache.Get(t.Context(), client, directory, row)
	require.NoError(t, err)
	assert.Equal(t, b.Bytes(), first)
	_, err = cache.Get(t.Context(), client, directory, row)
	require.NoError(t, err)
	assert.EqualValues(t, 1, calls.Load())
	require.NoError(t, cache.Cleanup(t.Context(), directory, -time.Second))
	entries, err := os.ReadDir(directory)
	require.NoError(t, err)
	assert.Empty(t, entries)
	require.Error(t, validJPEG([]byte("not an image")))
	row.Instance = "different"
	_, err = cache.Get(t.Context(), client, directory, row)
	require.Error(t, err)
	cfg.SnapshotImport = false
	client, err = NewClient(&cfg, nil)
	require.NoError(t, err)
	_, err = cache.Get(t.Context(), client, directory, row)
	require.Error(t, err)
}

func TestFrigateNormalizationAmbiguous(t *testing.T) {
	t.Parallel()
	names := speciesindex.Build([]string{"One species_Same name", "Other species_Same name"}, nil, "en")
	svc := NewService(nil, nil, func() *speciesindex.Snapshot { return names }, nil, nil)
	row := &entities.VisualDetection{OriginalLabel: new("Same name"), ClassificationConfidence: new(0.9)}
	svc.normalize(row, 0)
	assert.Nil(t, row.ScientificName)
}

func TestFrigateDisabledMakesNoRequests(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); _, _ = w.Write([]byte("[]")) }))
	defer server.Close()
	cfg := config(server.URL)
	cfg.Enabled = false
	svc := NewService(testRepository(t, ""), func() conf.FrigateSettings { return cfg }, nil, nil, nil)
	require.NoError(t, svc.Sync(t.Context(), &cfg))
	assert.Zero(t, calls.Load())
}

func TestFrigateNativeAuthenticationAndRetry(t *testing.T) {
	t.Parallel()
	var logins, events atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/login" {
			var payload struct {
				User     string `json:"user"`
				Password string `json:"password"`
			}
			if json.NewDecoder(r.Body).Decode(&payload) != nil || payload.User != "user" || payload.Password != "secret" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			logins.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "frigate_token", Value: "session-token", HttpOnly: true})
			return
		}
		cookie, err := r.Cookie("frigate_token")
		if err != nil || cookie.Value != "session-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if events.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()
	cfg := config(server.URL)
	cfg.Username = "user"
	cfg.Password = "secret"
	cfg.MaxAttempts = 2
	client, err := NewClient(&cfg, nil)
	require.NoError(t, err)
	require.NoError(t, client.TestConnection(t.Context()))
	assert.EqualValues(t, 1, logins.Load())
	assert.EqualValues(t, 2, events.Load())
}

func TestFrigateDisableCancelsInflightPoll(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	canceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(canceled) }))
	defer server.Close()
	cfg := config(server.URL)
	cfg.RequestTimeout = 30
	var current atomic.Pointer[conf.FrigateSettings]
	current.Store(&cfg)
	svc := NewService(testRepository(t, ""), func() conf.FrigateSettings { return *current.Load() }, nil, nil, nil)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); svc.Run(ctx) }()
	const timeout = 5 * time.Second
	testutil.WaitForChannel(t, started, timeout, "poll did not start")
	disabled := cfg
	disabled.Enabled = false
	current.Store(&disabled)
	testutil.WaitForChannel(t, canceled, timeout, "disable did not cancel the in-flight request")
	cancel()
	testutil.WaitForChannel(t, done, timeout, "worker did not stop")
	assert.False(t, svc.Status().Enabled)
}
