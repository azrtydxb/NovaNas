package client

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ExitNotImplemented is the exit code used when the server returns 501.
const ExitNotImplemented = 3

// ErrNotImplemented is returned when the server responds 501.
var ErrNotImplemented = errors.New("server not yet implemented")

// Client is a minimal HTTP client for the NovaNas API.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// New creates a client for the given base URL. If insecure is true the client
// skips TLS verification.
func New(baseURL, token string, insecure bool) *Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure}, //nolint:gosec
	}
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Token:      token,
		HTTPClient: &http.Client{Transport: tr, Timeout: 30 * time.Second},
	}
}

// Do performs an HTTP request against the API. body may be nil; if non-nil it
// is marshalled as JSON. The decoded response body is written into out if
// non-nil. A 501 response returns ErrNotImplemented.
func (c *Client) Do(method, path string, body, out any) error {
	u, err := url.Parse(c.BaseURL + path)
	if err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, u.String(), reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotImplemented {
		return ErrNotImplemented
	}
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		apiErr := &APIError{Status: resp.StatusCode, Message: string(data)}
		_ = json.Unmarshal(data, apiErr)
		if apiErr.Message == "" {
			apiErr.Message = fmt.Sprintf("http %d", resp.StatusCode)
		}
		return apiErr
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// PostForm sends a form-encoded POST and decodes the response as JSON.
func (c *Client) PostForm(path string, form url.Values, out any) error {
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var tErr TokenError
		_ = json.Unmarshal(data, &tErr)
		if tErr.Code != "" {
			return &tErr
		}
		return &APIError{Status: resp.StatusCode, Message: string(data)}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

// Error implements error for TokenError.
func (e *TokenError) Error() string {
	if e.ErrorDescription != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.ErrorDescription)
	}
	return e.Code
}
