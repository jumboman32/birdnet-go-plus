package conf

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestFrigateSettingsSecretsAndClone(t *testing.T) {
	t.Parallel()
	s := &Settings{Realtime: RealtimeSettings{Frigate: FrigateSettings{Username: "private-user", Password: "private-password", Token: "private-token", Cameras: []string{"feeder"}}}}
	b, err := json.Marshal(s)
	require.NoError(t, err)
	assert.NotContains(t, string(b), "private-user")
	assert.NotContains(t, string(b), "private-password")
	assert.NotContains(t, string(b), "private-token")
	y, err := yaml.Marshal(s.Realtime.Frigate)
	require.NoError(t, err)
	var loaded FrigateSettings
	require.NoError(t, yaml.Unmarshal(y, &loaded))
	assert.Equal(t, s.Realtime.Frigate, loaded)
	clone := CloneSettings(s)
	clone.Realtime.Frigate.Cameras[0] = "changed"
	assert.Equal(t, "feeder", s.Realtime.Frigate.Cameras[0])
}

func TestFrigateSettingsValidation(t *testing.T) {
	t.Parallel()
	require.NoError(t, (&FrigateSettings{}).Validate())
	for _, base := range []string{"file:///etc/passwd", "http://user:secret@frigate:5000", "http://frigate?secret=value", "http://frigate/#secret"} {
		cfg := FrigateSettings{Enabled: true, BaseURL: base}
		require.Error(t, cfg.Validate())
	}
}
