package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/tphakala/birdnet-go/internal/errors"
	"gorm.io/gorm"
)

const confidenceBinCount = 20

// ConfidenceAnalyticsRepository aggregates recorded primary identification scores in the v2 schema.
// Secondary predictions are deliberately excluded: one saved detection contributes exactly once.
type ConfidenceAnalyticsRepository interface {
	Distribution(context.Context, int64, int64, []string, []uint) ([]ConfidenceSeries, error)
	Review(context.Context, int64, int64, string, uint, int, int) ([]ConfidenceReview, error)
}

// ConfidenceSeries describes one model's distribution, optionally for one species.
type ConfidenceSeries struct {
	ModelID       uint                      `json:"modelId"`
	Model         string                    `json:"model"`
	Species       string                    `json:"species"`
	CommonName    string                    `json:"commonName"`
	Count         int64                     `json:"count"`
	Bins          [confidenceBinCount]int64 `json:"bins"`
	Average       float64                   `json:"average"`
	Median        float64                   `json:"median"`
	Low           int64                     `json:"low"`
	Middle        int64                     `json:"middle"`
	High          int64                     `json:"high"`
	UniqueSpecies int                       `json:"uniqueSpecies"`
	TopSpecies    string                    `json:"topSpecies"`
	TopCommonName string                    `json:"topCommonName"`
	TopCount      int64                     `json:"topCount"`
}

// ConfidenceReview is a bounded review row from the exact same population as the histogram.
type ConfidenceReview struct {
	ID         uint    `json:"id"`
	Species    string  `json:"species"`
	CommonName string  `json:"commonName"`
	Confidence float64 `json:"confidence"`
	DetectedAt int64   `json:"detectedAt"`
}

type confidenceAnalyticsRepository struct{ *detectionRepository }

// NewConfidenceAnalyticsRepository creates read-only analytics for either v2 table naming mode.
func NewConfidenceAnalyticsRepository(db *gorm.DB, useV2Prefix bool) ConfidenceAnalyticsRepository {
	return &confidenceAnalyticsRepository{&detectionRepository{db: db, useV2Prefix: useV2Prefix}}
}

func (r *confidenceAnalyticsRepository) base(ctx context.Context, start, end int64, species []string, models []uint) *gorm.DB {
	q := r.buildAnalyticsBaseQuery(ctx, start, end, nil, nil).
		Joins(fmt.Sprintf("LEFT JOIN %s l ON l.id = d.label_id", r.labelsTable())).
		Joins(fmt.Sprintf("LEFT JOIN %s m ON m.id = d.model_id", r.modelsTable())).
		Where("d.confidence >= ? AND d.confidence <= ?", 0, 1)
	if len(species) > 0 {
		q = q.Where("l.scientific_name IN ?", species)
	}
	if len(models) > 0 {
		q = q.Where("d.model_id IN ?", models)
	}
	return q
}

type confidenceBucket struct {
	ModelID uint
	Model   string
	Species string
	Bin     int
	Count   int64
	Sum     float64
}

// Distribution groups in SQL, so neither raw detections nor their scores are loaded into memory.
// With no species filter there is one row per recorded model; otherwise each species/model pair
// is kept separate. Exact medians load at most two ordered scores for each returned series.
func (r *confidenceAnalyticsRepository) Distribution(ctx context.Context, start, end int64, species []string, models []uint) ([]ConfidenceSeries, error) {
	var result []ConfidenceSeries
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		reader := &confidenceAnalyticsRepository{&detectionRepository{db: tx, useV2Prefix: r.useV2Prefix}}
		var err error
		result, err = reader.distribution(ctx, start, end, species, models)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return result, err
}

func (r *confidenceAnalyticsRepository) distribution(ctx context.Context, start, end int64, species []string, models []uint) ([]ConfidenceSeries, error) {
	var buckets []confidenceBucket
	// Compare the same boundaries used by Review. Multiplication can round a score
	// just below a boundary into the next bin, making its review page disagree.
	var binSQL strings.Builder
	binSQL.WriteString("CASE")
	for bin := range confidenceBinCount - 1 {
		fmt.Fprintf(&binSQL, " WHEN d.confidence < %.2f THEN %d", float64(bin+1)/confidenceBinCount, bin)
	}
	fmt.Fprintf(&binSQL, " ELSE %d END", confidenceBinCount-1)
	binExpr := binSQL.String()
	err := r.base(ctx, start, end, species, models).
		Select("d.model_id, COALESCE(m.name, '') AS model, COALESCE(l.scientific_name, '') AS species, " + binExpr + " AS bin, COUNT(*) AS count, SUM(d.confidence) AS sum").
		Group("d.model_id, m.name, l.scientific_name, " + binExpr).Scan(&buckets).Error
	if err != nil {
		return nil, err
	}
	type key struct {
		model   uint
		species string
	}
	series := make(map[key]*ConfidenceSeries)
	counts := make(map[key]map[string]int64)
	for _, b := range buckets {
		k := key{model: b.ModelID}
		if len(species) > 0 {
			k.species = b.Species
		}
		if series[k] == nil {
			series[k] = &ConfidenceSeries{ModelID: b.ModelID, Model: b.Model, Species: k.species}
			counts[k] = make(map[string]int64)
		}
		s := series[k]
		s.Count += b.Count
		s.Average += b.Sum
		s.Bins[b.Bin] += b.Count
		counts[k][b.Species] += b.Count
		// Bins have half-open bounds, except the last bin includes 100%.
		const lowStart, middleStart, highStart = 4, 6, 16
		switch {
		case b.Bin >= highStart:
			s.High += b.Count
		case b.Bin >= middleStart:
			s.Middle += b.Count
		case b.Bin >= lowStart:
			s.Low += b.Count
		}
	}
	result := make([]ConfidenceSeries, 0, len(series))
	for k, s := range series {
		s.Average /= float64(s.Count)
		for name, count := range counts[k] {
			if name != "" {
				s.UniqueSpecies++
			}
			if count > s.TopCount || (count == s.TopCount && name < s.TopSpecies) {
				s.TopSpecies, s.TopCount = name, count
			}
		}
		filter := []string(nil)
		if s.Species != "" {
			filter = []string{s.Species}
		}
		var scores []float64
		medianRows := 1
		if s.Count%2 == 0 {
			medianRows = 2
		}
		if err := r.base(ctx, start, end, filter, []uint{s.ModelID}).Order("d.confidence").Offset(int((s.Count-1)/2)).Limit(medianRows).Pluck("d.confidence", &scores).Error; err != nil {
			return nil, err
		}
		// Guard the snapshot invariant rather than fabricating a median from missing scores.
		if len(scores) != medianRows {
			return nil, errors.NewStd("confidence population changed; retry analytics")
		}
		for _, score := range scores {
			s.Median += score / float64(medianRows)
		}
		result = append(result, *s)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ModelID != result[j].ModelID {
			return result[i].ModelID < result[j].ModelID
		}
		return result[i].Species < result[j].Species
	})
	return result, nil
}

// Review loads only one page, excluding false positives and using half-open confidence bins.
func (r *confidenceAnalyticsRepository) Review(ctx context.Context, start, end int64, species string, model uint, bin, offset int) ([]ConfidenceReview, error) {
	const reviewPageSize = 25
	filter := []string(nil)
	if species != "" {
		filter = []string{species}
	}
	q := r.base(ctx, start, end, filter, []uint{model}).Where("d.confidence >= ?", float64(bin)/confidenceBinCount)
	if bin < confidenceBinCount-1 {
		q = q.Where("d.confidence < ?", float64(bin+1)/confidenceBinCount)
	}
	rows := make([]ConfidenceReview, 0, reviewPageSize)
	err := q.Select("d.id, COALESCE(l.scientific_name, '') AS species, d.confidence, d.detected_at").Order("d.detected_at DESC, d.id DESC").Offset(offset).Limit(reviewPageSize).Scan(&rows).Error
	return rows, err
}
