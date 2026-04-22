package client

import (
	"errors"
	"net/url"
	"time"

	"github.com/zalando/go-keyring"
)

// KeyringService is the OS keyring service name under which refresh tokens
// are stored.
const KeyringService = "novanasctl"

// RequestDeviceCode asks the NovaNas API to start a device-code flow.
func (c *Client) RequestDeviceCode() (*DeviceCodeResponse, error) {
	var r DeviceCodeResponse
	if err := c.Do("POST", "/api/v1/auth/device-code", nil, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// PollToken polls the token endpoint until the user completes the flow or the
// device code expires.
func (c *Client) PollToken(deviceCode string, interval, expiresIn int) (*TokenResponse, error) {
	if interval <= 0 {
		interval = 5
	}
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)
	for time.Now().Before(deadline) {
		form := url.Values{}
		form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		form.Set("device_code", deviceCode)

		var tok TokenResponse
		err := c.PostForm("/api/v1/auth/token", form, &tok)
		if err == nil {
			return &tok, nil
		}
		var tErr *TokenError
		if errors.As(err, &tErr) {
			switch tErr.Code {
			case "authorization_pending":
				// keep polling
			case "slow_down":
				interval += 5
			default:
				return nil, err
			}
		} else {
			return nil, err
		}
		time.Sleep(time.Duration(interval) * time.Second)
	}
	return nil, errors.New("device code expired before authorization completed")
}

// StoreRefreshToken saves a refresh token in the OS keyring, keyed by context.
func StoreRefreshToken(contextName, refreshToken string) error {
	return keyring.Set(KeyringService, contextName, refreshToken)
}

// LoadRefreshToken fetches a refresh token from the OS keyring.
func LoadRefreshToken(contextName string) (string, error) {
	return keyring.Get(KeyringService, contextName)
}

// DeleteRefreshToken removes a stored refresh token.
func DeleteRefreshToken(contextName string) error {
	return keyring.Delete(KeyringService, contextName)
}

// RefreshAccessToken exchanges a refresh token for a new access token.
func (c *Client) RefreshAccessToken(refreshToken string) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	var tok TokenResponse
	if err := c.PostForm("/api/v1/auth/token", form, &tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

// WhoAmI calls /api/v1/auth/whoami.
func (c *Client) WhoAmI() (*WhoAmI, error) {
	var w WhoAmI
	if err := c.Do("GET", "/api/v1/auth/whoami", nil, &w); err != nil {
		return nil, err
	}
	return &w, nil
}
