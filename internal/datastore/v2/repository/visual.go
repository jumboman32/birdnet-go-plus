package repository

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/tphakala/birdnet-go/internal/datastore"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/entities"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// VisualFilter restricts visual observation queries. End is exclusive.
type VisualFilter struct {
	Start, End           *time.Time
	Camera, Species      string
	Query                string
	MinConfidence        *float64
	Identified, Snapshot *bool
	Limit, Offset        int
}

// VisualRepository persists camera observations without touching audio records.
type VisualRepository interface {
	Upsert(context.Context, []entities.VisualDetection) error
	Cursor(context.Context, string) (time.Time, error)
	SetCursor(context.Context, string, time.Time) error
	Get(context.Context, uint) (*entities.VisualDetection, error)
	List(context.Context, *VisualFilter) ([]entities.VisualDetection, int64, error)
	ReviewSpecies(context.Context, uint, string, string, bool) error
	Summary(context.Context, time.Time, time.Time) (*VisualSummary, error)
}

// VisualSummary contains bounded aggregates; hours are UTC.
type VisualSummary struct {
	Total         int64                `json:"total"`
	UniqueSpecies int64                `json:"uniqueSpecies"`
	Unidentified  int64                `json:"unidentified"`
	Today         int64                `json:"today"`
	Species       []VisualSpeciesCount `json:"species"`
	Hours         []VisualHourCount    `json:"hours"`
}

// VisualSpeciesCount counts one resolved species.
type VisualSpeciesCount struct {
	ScientificName string `json:"scientificName"`
	Count          int64  `json:"count"`
}

// VisualHourCount counts observations in a UTC hour of day.
type VisualHourCount struct {
	Hour  int   `json:"hour"`
	Count int64 `json:"count"`
}

type visualRepository struct {
	db     *gorm.DB
	prefix string
}

const (
	visualTable       = "visual_detections"
	visualCursorTable = "visual_sync_states"
	visualMaxPage     = 200
	visualTopSpecies  = 20
)

func mustZones(zones []string) string { b, _ := json.Marshal(zones); return string(b) }

// Numeric instants bypass SQL driver and server time zone conversion, including
// DST gaps. Public time fields are derived from these persisted Unix microseconds.
func visualSQLTime(t time.Time) int64 { return t.UnixMicro() }

func visualSQLTimePointer(t *time.Time) any {
	if t == nil {
		return nil
	}
	return visualSQLTime(*t)
}

func (r *visualRepository) restoreUTC(row *entities.VisualDetection) {
	if row.ClassificationSource == "" && row.OriginalLabel != nil {
		row.ClassificationSource = entities.VisualClassificationFrigate
	}
	row.StartTime = time.UnixMicro(row.StartUnixMicro).UTC()
	if row.EndUnixMicro != nil {
		end := time.UnixMicro(*row.EndUnixMicro).UTC()
		row.EndTime = &end
	}
	row.CreatedAt = time.UnixMicro(row.CreatedUnixMicro).UTC()
	row.UpdatedAt = time.UnixMicro(row.UpdatedUnixMicro).UTC()
}

func visualValues(row *entities.VisualDetection) map[string]any {
	now := visualSQLTime(time.Now())
	return map[string]any{"source": row.Source, "instance": row.Instance, "external_event_id": row.ExternalEventID, "camera": row.Camera, "object_label": row.ObjectLabel, "original_label": row.OriginalLabel, "common_name": row.CommonName, scientificNameColumn: row.ScientificName, "detection_confidence": row.DetectionConfidence, "classification_confidence": row.ClassificationConfidence, "classification_source": row.ClassificationSource, "start_time": visualSQLTime(row.StartTime), "end_time": visualSQLTimePointer(row.EndTime), "zones": mustZones(row.Zones), "snapshot_status": row.SnapshotStatus, "clip_available": row.ClipAvailable, "false_positive": row.FalsePositive, "created_at": now, "updated_at": now}
}

// NewVisualRepository uses the running v2 manager's naming mode for both dialects.
func NewVisualRepository(db *gorm.DB, prefix string) VisualRepository {
	return &visualRepository{db: db, prefix: prefix}
}

func (r *visualRepository) Upsert(ctx context.Context, rows []entities.VisualDetection) error {
	if len(rows) == 0 {
		return nil
	}
	return datastore.RetryOnLock(ctx, "visual_upsert", func() error {
		return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for i := range rows {
				// Nullable classification fields are retained if a later response omits them.
				updates := map[string]any{"camera": rows[i].Camera, "object_label": rows[i].ObjectLabel, "start_time": visualSQLTime(rows[i].StartTime), "snapshot_status": rows[i].SnapshotStatus, "clip_available": rows[i].ClipAvailable, "updated_at": visualSQLTime(time.Now())}
				if rows[i].Zones != nil {
					updates["zones"] = mustZones(rows[i].Zones)
				}
				if rows[i].OriginalLabel != nil {
					updates["classification_source"] = entities.VisualClassificationFrigate
					updates["original_label"] = rows[i].OriginalLabel
					// A changed species label must never retain the previous species identity.
					for _, column := range []string{scientificNameColumn, "common_name", "classification_confidence"} {
						updates[column] = clause.Expr{SQL: "CASE WHEN original_label = ? THEN " + column + " ELSE NULL END", Vars: []any{*rows[i].OriginalLabel}}
					}
				}
				if rows[i].ScientificName != nil {
					updates[scientificNameColumn] = rows[i].ScientificName
					updates["common_name"] = rows[i].CommonName
				}
				if rows[i].DetectionConfidence != nil {
					updates["detection_confidence"] = rows[i].DetectionConfidence
				}
				if rows[i].ClassificationConfidence != nil {
					updates["classification_confidence"] = rows[i].ClassificationConfidence
				}
				if rows[i].EndTime != nil {
					updates["end_time"] = visualSQLTime(*rows[i].EndTime)
				}
				if rows[i].FalsePositive != nil {
					updates["false_positive"] = rows[i].FalsePositive
				}
				// Owner-confirmed identity is independent of Frigate's provisional label.
				// Keep importing event metadata while protecting the reviewed species.
				assignments := visualAssignments(updates)
				if err := tx.Table(r.prefix + visualTable).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "source"}, {Name: "instance"}, {Name: "external_event_id"}}, DoUpdates: assignments}).Create(visualValues(&rows[i])).Error; err != nil {
					return err
				}
			}
			return nil
		})
	}, nil)
}

// visualAssignments keeps owner reviews intact and assigns original_label last:
// MySQL evaluates assignments from left to right.
func visualAssignments(updates map[string]any) clause.Set {
	for _, column := range []string{scientificNameColumn, "common_name", "classification_confidence", "classification_source"} {
		if value, exists := updates[column]; exists {
			updates[column] = clause.Expr{SQL: "CASE WHEN species_verified = ? THEN " + column + " ELSE ? END", Vars: []any{true, value}}
		}
	}
	assignments := clause.Assignments(updates)
	for j := range assignments {
		if assignments[j].Column.Name == "original_label" {
			item := assignments[j]
			assignments = append(append(assignments[:j:j], assignments[j+1:]...), item)
			break
		}
	}
	return assignments
}

// ReviewSpecies confirms an owner-supplied identity or returns the event to automatic classification.
func (r *visualRepository) ReviewSpecies(ctx context.Context, id uint, scientificName, commonName string, verified bool) error {
	updates := map[string]any{"species_verified": verified, "updated_at": visualSQLTime(time.Now())}
	updates[scientificNameColumn] = nil
	updates["common_name"] = nil
	updates["classification_confidence"] = nil
	updates["classification_source"] = entities.VisualClassificationFrigate
	if verified {
		updates[scientificNameColumn] = scientificName
		updates["common_name"] = commonName
		updates["classification_confidence"] = nil
		updates["classification_source"] = entities.VisualClassificationManual
	}
	result := r.db.WithContext(ctx).Table(r.prefix+visualTable).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *visualRepository) Cursor(ctx context.Context, key string) (time.Time, error) {
	var state entities.VisualSyncState
	err := r.db.WithContext(ctx).Table(r.prefix+visualCursorTable).Where("`key` = ?", key).Find(&state).Error
	if state.Through == 0 {
		return time.Time{}, err
	}
	return time.UnixMicro(state.Through).UTC(), err
}

func (r *visualRepository) SetCursor(ctx context.Context, key string, through time.Time) error {
	state := map[string]any{"key": key, "through": visualSQLTime(through)}
	return r.db.WithContext(ctx).Table(r.prefix + visualCursorTable).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"through"})}).Create(state).Error
}

func (r *visualRepository) Get(ctx context.Context, id uint) (*entities.VisualDetection, error) {
	var row entities.VisualDetection
	err := r.db.WithContext(ctx).Table(r.prefix+visualTable).First(&row, id).Error
	r.restoreUTC(&row)
	return &row, err
}

func (r *visualRepository) query(ctx context.Context, f *VisualFilter) *gorm.DB {
	q := r.db.WithContext(ctx).Table(r.prefix + visualTable)
	if f.Start != nil {
		q = q.Where("start_time >= ?", visualSQLTime(*f.Start))
	}
	if f.End != nil {
		q = q.Where("start_time < ?", visualSQLTime(*f.End))
	}
	if f.Camera != "" {
		q = q.Where("camera = ?", f.Camera)
	}
	if f.Species != "" {
		q = q.Where("scientific_name = ?", f.Species)
	}
	if f.Query != "" {
		pattern := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.ToLower(f.Query)) + "%"
		q = q.Where("LOWER(common_name) LIKE ? ESCAPE '!' OR LOWER(scientific_name) LIKE ? ESCAPE '!' OR LOWER(original_label) LIKE ? ESCAPE '!' OR LOWER(camera) LIKE ? ESCAPE '!'", pattern, pattern, pattern, pattern)
	}
	if f.MinConfidence != nil {
		q = q.Where("classification_confidence >= ?", f.MinConfidence)
	}
	if f.Identified != nil {
		if *f.Identified {
			q = q.Where("scientific_name IS NOT NULL")
		} else {
			q = q.Where("scientific_name IS NULL")
		}
	}
	if f.Snapshot != nil {
		if *f.Snapshot {
			q = q.Where("snapshot_status = ?", "available")
		} else {
			q = q.Where("snapshot_status <> ?", "available")
		}
	}
	return q
}

func (r *visualRepository) List(ctx context.Context, filters *VisualFilter) ([]entities.VisualDetection, int64, error) {
	f := VisualFilter{}
	if filters != nil {
		f = *filters
	}
	var rows []entities.VisualDetection
	var total int64
	if f.Limit <= 0 || f.Limit > visualMaxPage {
		f.Limit = visualMaxPage
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	if err := r.query(ctx, &f).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := r.query(ctx, &f).Order("start_time DESC, id DESC").Limit(f.Limit).Offset(f.Offset).Find(&rows).Error
	for i := range rows {
		r.restoreUTC(&rows[i])
	}
	return rows, total, err
}

func (r *visualRepository) Summary(ctx context.Context, start, end time.Time) (*VisualSummary, error) {
	s := &VisualSummary{Species: []VisualSpeciesCount{}, Hours: []VisualHourCount{}}
	q := func() *gorm.DB { return r.db.WithContext(ctx).Table(r.prefix + visualTable) }
	if err := q().Count(&s.Total).Error; err != nil {
		return nil, err
	}
	if err := q().Where("scientific_name IS NOT NULL").Distinct("scientific_name").Count(&s.UniqueSpecies).Error; err != nil {
		return nil, err
	}
	if err := q().Where("scientific_name IS NULL").Count(&s.Unidentified).Error; err != nil {
		return nil, err
	}
	if err := q().Where("start_time >= ? AND start_time < ?", visualSQLTime(start), visualSQLTime(end)).Count(&s.Today).Error; err != nil {
		return nil, err
	}
	if err := q().Select("scientific_name, COUNT(*) AS count").Where("scientific_name IS NOT NULL").Group("scientific_name").Order("count DESC, scientific_name ASC").Limit(visualTopSpecies).Scan(&s.Species).Error; err != nil {
		return nil, err
	}
	const hoursPerDay = 24
	divisor := strconv.FormatInt(int64(time.Hour/time.Microsecond), 10)
	hour := "(start_time / " + divisor + ") % " + strconv.Itoa(hoursPerDay)
	if r.db.Name() == "mysql" {
		hour = "(start_time DIV " + divisor + ") % " + strconv.Itoa(hoursPerDay)
	}
	if err := q().Select(hour + " AS hour, COUNT(*) AS count").Group(hour).Order("hour ASC").Scan(&s.Hours).Error; err != nil {
		return nil, err
	}
	return s, nil
}
