package frigate

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/tphakala/birdnet-go/internal/errors"
)

// login follows Frigate's native HTTP-only JWT cookie authentication. A fresh
// operation obtains a new session, so an expired token cannot strand polling.
func (c *Client) login(ctx context.Context) error {
	b, err := json.Marshal(struct {
		User     string `json:"user"`
		Password string `json:"password"`
	}{User: c.settings.Username, Password: c.settings.Password})
	if err != nil {
		return errors.NewStd("cannot encode Frigate login")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.settings.BaseURL, "/")+"/api/login", bytes.NewReader(b))
	if err != nil {
		return errors.NewStd("cannot construct Frigate login")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.NewStd("Frigate login connection failed or timed out")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return &HTTPError{Status: resp.StatusCode}
	}
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "frigate_token" && cookie.Value != "" {
			c.token = cookie.Value
			return nil
		}
	}
	return errors.NewStd("Frigate login did not return a session cookie")
}
