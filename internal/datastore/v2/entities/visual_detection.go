package entities

import "time"

const (
	// VisualClassificationFrigate identifies a label received through Frigate's sublabel API.
	VisualClassificationFrigate = "frigate_sublabel"
	// VisualClassificationManual identifies an owner-confirmed species assignment.
	VisualClassificationManual = "manual"
)

// VisualDetection is a camera observation, independent of audio detection counts.
// Confidence values are nullable: an absent score is never invented.
type VisualDetection struct {
	ID                       uint       `gorm:"primaryKey" json:"id"`
	Source                   string     `gorm:"size:20;not null;uniqueIndex:idx_visual_external" json:"source"`
	Instance                 string     `gorm:"size:64;not null;uniqueIndex:idx_visual_external" json:"-"`
	ExternalEventID          string     `gorm:"size:200;not null;uniqueIndex:idx_visual_external" json:"externalEventID"`
	Camera                   string     `gorm:"size:200;not null;index" json:"camera"`
	ObjectLabel              string     `gorm:"size:100;not null" json:"objectLabel"`
	OriginalLabel            *string    `gorm:"size:500" json:"originalLabel"`
	CommonName               *string    `gorm:"size:500" json:"commonName"`
	ScientificName           *string    `gorm:"size:200;index;index:idx_visual_species_time" json:"scientificName"`
	DetectionConfidence      *float64   `json:"detectionConfidence"`
	ClassificationConfidence *float64   `json:"classificationConfidence"`
	ClassificationSource     string     `gorm:"size:50;not null;default:''" json:"classificationSource"`
	SpeciesVerified          bool       `gorm:"not null;default:false" json:"speciesVerified"`
	StartTime                time.Time  `gorm:"-" json:"startTime"`
	EndTime                  *time.Time `gorm:"-" json:"endTime"`
	// Unix microseconds are the stored source of truth, independent of SQL connection zones.
	StartUnixMicro   int64     `gorm:"column:start_time;not null;index;index:idx_visual_species_time" json:"-"`
	EndUnixMicro     *int64    `gorm:"column:end_time" json:"-"`
	Zones            []string  `gorm:"serializer:json;type:text" json:"zones"`
	SnapshotStatus   string    `gorm:"size:20;not null" json:"snapshotStatus"`
	ClipAvailable    bool      `json:"clipAvailable"`
	FalsePositive    *bool     `json:"falsePositive"`
	CreatedAt        time.Time `gorm:"-" json:"createdAt"`
	UpdatedAt        time.Time `gorm:"-" json:"updatedAt"`
	CreatedUnixMicro int64     `gorm:"column:created_at" json:"-"`
	UpdatedUnixMicro int64     `gorm:"column:updated_at" json:"-"`
}

// VisualSyncState is a durable per-instance, per-camera-selection import cursor.
type VisualSyncState struct {
	Key     string `gorm:"primaryKey;size:64"`
	Through int64  // Unix microseconds; zero means no completed scan.
}
