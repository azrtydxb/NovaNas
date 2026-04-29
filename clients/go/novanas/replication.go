package novanas

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ReplicationSource mirrors the ReplicationSource schema in
// api/openapi.yaml. Only the fields meaningful to the chosen backend +
// direction need to be set.
type ReplicationSource struct {
	Dataset  string `json:"dataset,omitempty"`
	Path     string `json:"path,omitempty"`
	Host     string `json:"host,omitempty"`
	SSHUser  string `json:"sshUser,omitempty"`
	Bucket   string `json:"bucket,omitempty"`
	Prefix   string `json:"prefix,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	Region   string `json:"region,omitempty"`
}

// ReplicationDestination has the same shape as ReplicationSource on the
// receiving side.
type ReplicationDestination = ReplicationSource

// RetentionPolicy mirrors the RetentionPolicy schema. Zero values mean
// "no limit" for that bucket.
type RetentionPolicy struct {
	KeepLastN   int `json:"keepLastN,omitempty"`
	KeepDaily   int `json:"keepDaily,omitempty"`
	KeepWeekly  int `json:"keepWeekly,omitempty"`
	KeepMonthly int `json:"keepMonthly,omitempty"`
	KeepYearly  int `json:"keepYearly,omitempty"`
}

// ReplicationJob mirrors the ReplicationJob schema.
type ReplicationJob struct {
	ID           string                 `json:"id,omitempty"`
	Name         string                 `json:"name"`
	Backend      string                 `json:"backend"`   // zfs|s3|rsync
	Direction    string                 `json:"direction"` // push|pull
	Source       ReplicationSource      `json:"source"`
	Destination  ReplicationDestination `json:"destination"`
	Schedule     string                 `json:"schedule,omitempty"`
	Retention    RetentionPolicy        `json:"retention,omitempty"`
	Enabled      *bool                  `json:"enabled,omitempty"`
	SecretRef    string                 `json:"secretRef,omitempty"`
	LastSnapshot string                 `json:"lastSnapshot,omitempty"`
	CreatedAt    time.Time              `json:"createdAt,omitempty"`
	UpdatedAt    time.Time              `json:"updatedAt,omitempty"`
}

// ReplicationRun mirrors the ReplicationRun schema.
type ReplicationRun struct {
	ID               string     `json:"id"`
	JobID            string     `json:"jobId"`
	StartedAt        time.Time  `json:"startedAt"`
	FinishedAt       *time.Time `json:"finishedAt,omitempty"`
	Outcome          string     `json:"outcome"`
	BytesTransferred int64      `json:"bytesTransferred"`
	Snapshot         string     `json:"snapshot,omitempty"`
	Error            string     `json:"error,omitempty"`
}

// JobDispatchResult is the small {"jobId":"..."} envelope returned by
// any endpoint that enqueues an async job (mirrors the server's
// writeDispatchResult).
type JobDispatchResult struct {
	JobID string `json:"jobId"`
}

// ReplicationJobDetail is the GET /replication-jobs/{id} envelope.
type ReplicationJobDetail struct {
	ReplicationJob
	Runs []ReplicationRun `json:"runs"`
}

// ReplicationRunsPage is the paginated /runs envelope.
type ReplicationRunsPage struct {
	Runs       []ReplicationRun `json:"runs"`
	NextCursor string           `json:"nextCursor,omitempty"`
}

// ListReplicationJobs returns every replication job on the server.
func (c *Client) ListReplicationJobs(ctx context.Context) ([]ReplicationJob, error) {
	var out []ReplicationJob
	if _, err := c.do(ctx, http.MethodGet, "/replication-jobs", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetReplicationJob returns the named job plus the most-recent runs.
func (c *Client) GetReplicationJob(ctx context.Context, id string) (*ReplicationJobDetail, error) {
	if id == "" {
		return nil, errors.New("novanas: replication job id is required")
	}
	var out ReplicationJobDetail
	if _, err := c.do(ctx, http.MethodGet, "/replication-jobs/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateReplicationJob creates a new job and returns the server's view.
func (c *Client) CreateReplicationJob(ctx context.Context, j ReplicationJob) (*ReplicationJob, error) {
	if j.Name == "" || j.Backend == "" || j.Direction == "" {
		return nil, errors.New("novanas: ReplicationJob requires Name, Backend and Direction")
	}
	var out ReplicationJob
	if _, err := c.do(ctx, http.MethodPost, "/replication-jobs", nil, j, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateReplicationJob applies a partial update to the named job.
func (c *Client) UpdateReplicationJob(ctx context.Context, id string, j ReplicationJob) (*ReplicationJob, error) {
	if id == "" {
		return nil, errors.New("novanas: replication job id is required")
	}
	var out ReplicationJob
	if _, err := c.do(ctx, http.MethodPatch, "/replication-jobs/"+url.PathEscape(id), nil, j, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteReplicationJob removes a job and its associated secrets.
func (c *Client) DeleteReplicationJob(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("novanas: replication job id is required")
	}
	_, err := c.do(ctx, http.MethodDelete, "/replication-jobs/"+url.PathEscape(id), nil, nil, nil)
	return err
}

// RunReplicationJob enqueues an immediate run and returns the dispatched
// job-id envelope (the same shape returned by other dispatching
// endpoints in this API).
func (c *Client) RunReplicationJob(ctx context.Context, id string) (*JobDispatchResult, error) {
	if id == "" {
		return nil, errors.New("novanas: replication job id is required")
	}
	var out JobDispatchResult
	if _, err := c.do(ctx, http.MethodPost, "/replication-jobs/"+url.PathEscape(id)+"/run", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListReplicationRuns returns the run history page for a job. cursor is
// opaque; pass an empty string for the first page and the value of the
// previous response's NextCursor to continue.
func (c *Client) ListReplicationRuns(ctx context.Context, id string, limit int, cursor string) (*ReplicationRunsPage, error) {
	if id == "" {
		return nil, errors.New("novanas: replication job id is required")
	}
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	var out ReplicationRunsPage
	if _, err := c.do(ctx, http.MethodGet, "/replication-jobs/"+url.PathEscape(id)+"/runs", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
