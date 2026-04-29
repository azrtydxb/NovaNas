package novanas

import (
	"context"
	"net/http"
	"time"
)

// SystemVersion mirrors the /api/v1/system/version response.
type SystemVersion struct {
	GoVersion string `json:"goVersion"`
	Commit    string `json:"commit,omitempty"`
	BuildTime string `json:"buildTime,omitempty"`
	Module    string `json:"module,omitempty"`
	Version   string `json:"version,omitempty"`
}

// SystemUpdate mirrors the /api/v1/system/updates response. v1 always
// returns Available=false until the OS image-update layer is built.
type SystemUpdate struct {
	Available        bool      `json:"available"`
	Reason           string    `json:"reason,omitempty"`
	CurrentVersion   string    `json:"currentVersion,omitempty"`
	AvailableVersion string    `json:"availableVersion,omitempty"`
	Channel          string    `json:"channel,omitempty"`
	LastChecked      time.Time `json:"lastChecked,omitempty"`
	Status           string    `json:"status,omitempty"`
}

// GetSystemVersion returns nova-api's build metadata.
func (c *Client) GetSystemVersion(ctx context.Context) (*SystemVersion, error) {
	var out SystemVersion
	if _, err := c.do(ctx, http.MethodGet, "/system/version", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetSystemUpdates returns the current image-update state.
func (c *Client) GetSystemUpdates(ctx context.Context) (*SystemUpdate, error) {
	var out SystemUpdate
	if _, err := c.do(ctx, http.MethodGet, "/system/updates", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
