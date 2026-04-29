package novanas

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
)

// LogQueryResponse mirrors Loki's /query and /query_range envelope.
// Data is left as a json.RawMessage because the inner shape varies by
// query type (matrix/streams/scalar).
type LogQueryResponse struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
}

// LogLabelsResponse mirrors Loki's /labels.
type LogLabelsResponse struct {
	Status string   `json:"status"`
	Data   []string `json:"data"`
}

// QueryLogsRange runs a LogQL range query.
func (c *Client) QueryLogsRange(ctx context.Context, query, start, end string, limit int) (*LogQueryResponse, error) {
	if query == "" {
		return nil, errors.New("novanas: query is required")
	}
	q := url.Values{}
	q.Set("query", query)
	if start != "" {
		q.Set("start", start)
	}
	if end != "" {
		q.Set("end", end)
	}
	if limit > 0 {
		q.Set("limit", itoa(limit))
	}
	var out LogQueryResponse
	if _, err := c.do(ctx, http.MethodGet, "/logs/query", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// QueryLogsInstant runs a LogQL instant query.
func (c *Client) QueryLogsInstant(ctx context.Context, query, atTime string) (*LogQueryResponse, error) {
	if query == "" {
		return nil, errors.New("novanas: query is required")
	}
	q := url.Values{}
	q.Set("query", query)
	if atTime != "" {
		q.Set("time", atTime)
	}
	var out LogQueryResponse
	if _, err := c.do(ctx, http.MethodGet, "/logs/query/instant", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListLogLabels returns all label keys known to Loki.
func (c *Client) ListLogLabels(ctx context.Context) (*LogLabelsResponse, error) {
	var out LogLabelsResponse
	if _, err := c.do(ctx, http.MethodGet, "/logs/labels", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListLogLabelValues returns all values for the given label key.
func (c *Client) ListLogLabelValues(ctx context.Context, name string) (*LogLabelsResponse, error) {
	if name == "" {
		return nil, errors.New("novanas: label name is required")
	}
	var out LogLabelsResponse
	if _, err := c.do(ctx, http.MethodGet, "/logs/labels/"+url.PathEscape(name)+"/values", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListLogSeries lists all streams matching the given label-matchers.
func (c *Client) ListLogSeries(ctx context.Context, matchers []string) (*LogQueryResponse, error) {
	q := url.Values{}
	for _, m := range matchers {
		q.Add("match[]", m)
	}
	var out LogQueryResponse
	if _, err := c.do(ctx, http.MethodGet, "/logs/series", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// itoa is a minimal local strconv.Itoa to avoid an extra import.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
