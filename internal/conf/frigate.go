package conf

import (
	"net/url"
	"strings"

	"github.com/tphakala/birdnet-go/internal/errors"
)

// FrigateSettings configures the optional visual observation importer. Credentials
// are server-only YAML settings and are never serialized to API clients.
type FrigateSettings struct {
	Enabled                 bool     `yaml:"enabled" json:"enabled"`
	BaseURL                 string   `yaml:"baseurl" json:"baseURL"`
	Username                string   `yaml:"username" json:"-"`
	Password                string   `yaml:"password" json:"-"`
	Token                   string   `yaml:"token" json:"-"`
	Cameras                 []string `yaml:"cameras" json:"cameras"`
	PollInterval            int      `yaml:"pollinterval" json:"pollInterval"`
	SnapshotImport          bool     `yaml:"snapshotimport" json:"snapshotImport"`
	InitialHistoryHours     int      `yaml:"initialhistoryhours" json:"initialHistoryHours"`
	RequestTimeout          int      `yaml:"requesttimeout" json:"requestTimeout"`
	MaxAttempts             int      `yaml:"maxattempts" json:"maxAttempts"`
	ClassificationThreshold float64  `yaml:"classificationthreshold" json:"classificationThreshold"`
	CorrelationWindow       int      `yaml:"correlationwindow" json:"correlationWindow"`
	OverlapSeconds          int      `yaml:"overlapseconds" json:"overlapSeconds"`
	SnapshotRetentionHours  int      `yaml:"snapshotretentionhours" json:"snapshotRetentionHours"`
}

// Validate checks resource bounds and rejects URLs carrying credentials or query strings.
func (s *FrigateSettings) Validate() error {
	if !s.Enabled && s.BaseURL == "" {
		return nil
	}
	u, err := url.Parse(s.BaseURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != SchemeHTTP && u.Scheme != SchemeHTTPS) || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.NewStd("frigate base URL must be an HTTP(S) URL without credentials, query or fragment")
	}
	if !s.validBounds() {
		return errors.NewStd("frigate settings contain out-of-range intervals, retries or confidence")
	}
	const maxCameras = 64
	if len(s.Cameras) == 0 || len(s.Cameras) > maxCameras {
		return errors.NewStd("frigate requires selected cameras")
	}
	for _, camera := range s.Cameras {
		if strings.TrimSpace(camera) == "" || strings.ContainsAny(camera, ",\r\n") {
			return errors.NewStd("frigate camera names must be nonempty and cannot contain commas or newlines")
		}
	}
	return nil
}

func (s *FrigateSettings) validBounds() bool {
	const (
		maxInterval = 3600
		maxHistory  = 87600
		maxAttempts = 8
	)
	if s.PollInterval < 1 || s.PollInterval > maxInterval || s.RequestTimeout < 1 || s.RequestTimeout > maxInterval || s.MaxAttempts < 1 || s.MaxAttempts > maxAttempts || s.InitialHistoryHours < 0 || s.InitialHistoryHours > maxHistory || s.OverlapSeconds < 1 || s.OverlapSeconds > maxHistory*maxInterval || s.SnapshotRetentionHours < 1 || s.SnapshotRetentionHours > maxHistory || s.CorrelationWindow < 1 || s.CorrelationWindow > maxInterval || !(s.ClassificationThreshold >= 0 && s.ClassificationThreshold <= 1) {
		return false
	}
	return true
}
