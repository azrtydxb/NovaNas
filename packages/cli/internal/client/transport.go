// Package client provides the HTTP client used to talk to the NovaNas API.
package client

import "time"

// DeviceCodeResponse is returned by POST /api/v1/auth/device-code.
type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// TokenResponse is returned by POST /api/v1/auth/token.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// TokenError is returned by the token endpoint while polling.
type TokenError struct {
	Code             string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// WhoAmI is a minimal user identity payload.
type WhoAmI struct {
	Subject   string    `json:"sub"`
	Email     string    `json:"email,omitempty"`
	Username  string    `json:"preferred_username,omitempty"`
	Groups    []string  `json:"groups,omitempty"`
	IssuedAt  time.Time `json:"iat,omitempty"`
	ExpiresAt time.Time `json:"exp,omitempty"`
}

// ListResult is a generic list wrapper for resource endpoints.
type ListResult struct {
	Items []map[string]any `json:"items"`
}

// APIError is the JSON error envelope returned by the API.
type APIError struct {
	Status  int    `json:"status"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}
