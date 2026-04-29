package novanas

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"
)

// ---- Pools ------------------------------------------------------------------

// ListPools returns all pools (GET /pools).
func (c *Client) ListPools(ctx context.Context) ([]Pool, error) {
	var out []Pool
	if _, err := c.do(ctx, http.MethodGet, "/pools", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetPool returns a single pool's detail (GET /pools/{name}).
func (c *Client) GetPool(ctx context.Context, name string) (*PoolDetail, error) {
	if name == "" {
		return nil, errors.New("novanas: pool name is required")
	}
	var out PoolDetail
	if _, err := c.do(ctx, http.MethodGet, "/pools/"+url.PathEscape(name), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---- Datasets ---------------------------------------------------------------

// ListDatasets returns datasets, optionally restricted to a pool. The server
// query parameter is "pool"; we expose it as `root` here because the CSI
// driver tends to think in terms of "the dataset rooted at X". Empty `root`
// returns all datasets.
func (c *Client) ListDatasets(ctx context.Context, root string) ([]Dataset, error) {
	var q url.Values
	if root != "" {
		q = url.Values{"pool": {root}}
	}
	var out []Dataset
	if _, err := c.do(ctx, http.MethodGet, "/datasets", q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetDataset returns a single dataset's detail.
func (c *Client) GetDataset(ctx context.Context, fullname string) (*DatasetDetail, error) {
	if fullname == "" {
		return nil, errors.New("novanas: dataset fullname is required")
	}
	var out DatasetDetail
	if _, err := c.do(ctx, http.MethodGet, "/datasets/"+url.PathEscape(fullname), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateDataset enqueues a dataset creation job and returns a Job stub
// containing the new job ID (state="queued"). Use WaitJob to block until
// it terminates.
func (c *Client) CreateDataset(ctx context.Context, spec CreateDatasetSpec) (*Job, error) {
	if spec.Parent == "" || spec.Name == "" || spec.Type == "" {
		return nil, errors.New("novanas: CreateDatasetSpec requires Parent, Name, Type")
	}
	if spec.Type == DatasetTypeVolume && spec.VolumeSizeBytes <= 0 {
		return nil, errors.New("novanas: volume datasets require VolumeSizeBytes > 0")
	}
	resp, err := c.do(ctx, http.MethodPost, "/datasets", nil, spec, nil)
	if err != nil {
		return nil, err
	}
	return finishJobFromAccepted(resp)
}

// DestroyDataset enqueues a dataset destroy job.
func (c *Client) DestroyDataset(ctx context.Context, fullname string, recursive bool) (*Job, error) {
	if fullname == "" {
		return nil, errors.New("novanas: dataset fullname is required")
	}
	q := url.Values{}
	if recursive {
		q.Set("recursive", boolQuery(true))
	}
	resp, err := c.do(ctx, http.MethodDelete, "/datasets/"+url.PathEscape(fullname), q, nil, nil)
	if err != nil {
		return nil, err
	}
	return finishJobFromAccepted(resp)
}

// SetDatasetProps enqueues a properties patch (PATCH /datasets/{fullname}).
func (c *Client) SetDatasetProps(ctx context.Context, fullname string, props map[string]string) (*Job, error) {
	if fullname == "" {
		return nil, errors.New("novanas: dataset fullname is required")
	}
	if len(props) == 0 {
		return nil, errors.New("novanas: at least one property is required")
	}
	body := struct {
		Properties map[string]string `json:"properties"`
	}{Properties: props}
	resp, err := c.do(ctx, http.MethodPatch, "/datasets/"+url.PathEscape(fullname), nil, body, nil)
	if err != nil {
		return nil, err
	}
	return finishJobFromAccepted(resp)
}

// RenameDataset enqueues a rename job. newName is the *new full name*
// (matching the server's DatasetRenameRequest.newName field).
func (c *Client) RenameDataset(ctx context.Context, fullname, newName string) (*Job, error) {
	if fullname == "" || newName == "" {
		return nil, errors.New("novanas: rename requires fullname and newName")
	}
	body := struct {
		NewName string `json:"newName"`
	}{NewName: newName}
	resp, err := c.do(ctx, http.MethodPost, "/datasets/"+url.PathEscape(fullname)+"/rename", nil, body, nil)
	if err != nil {
		return nil, err
	}
	return finishJobFromAccepted(resp)
}

// CloneSnapshot clones an existing snapshot into a new dataset. snapshot is
// the full snapshot name (e.g. "tank/data@snap1"), target is the new
// dataset name. properties is optional.
func (c *Client) CloneSnapshot(ctx context.Context, snapshot, target string, properties map[string]string) (*Job, error) {
	if snapshot == "" || target == "" {
		return nil, errors.New("novanas: clone requires snapshot and target")
	}
	body := struct {
		Target     string            `json:"target"`
		Properties map[string]string `json:"properties,omitempty"`
	}{Target: target, Properties: properties}
	// The server's clone route lives at /datasets/{fullname}/clone, where
	// {fullname} is the URL-encoded snapshot name.
	resp, err := c.do(ctx, http.MethodPost, "/datasets/"+url.PathEscape(snapshot)+"/clone", nil, body, nil)
	if err != nil {
		return nil, err
	}
	return finishJobFromAccepted(resp)
}

// ---- Snapshots --------------------------------------------------------------

// ListSnapshots returns snapshots, optionally restricted to a dataset.
// The server query parameter is "dataset".
func (c *Client) ListSnapshots(ctx context.Context, root string) ([]Snapshot, error) {
	var q url.Values
	if root != "" {
		q = url.Values{"dataset": {root}}
	}
	var out []Snapshot
	if _, err := c.do(ctx, http.MethodGet, "/snapshots", q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateSnapshot enqueues a snapshot creation. shortName is the part after
// '@'; the server combines it with dataset to form the full name.
func (c *Client) CreateSnapshot(ctx context.Context, dataset, shortName string, recursive bool) (*Job, error) {
	if dataset == "" || shortName == "" {
		return nil, errors.New("novanas: snapshot requires dataset and shortName")
	}
	body := struct {
		Dataset   string `json:"dataset"`
		Name      string `json:"name"`
		Recursive bool   `json:"recursive,omitempty"`
	}{Dataset: dataset, Name: shortName, Recursive: recursive}
	resp, err := c.do(ctx, http.MethodPost, "/snapshots", nil, body, nil)
	if err != nil {
		return nil, err
	}
	return finishJobFromAccepted(resp)
}

// DestroySnapshot enqueues snapshot destruction. fullname is the full
// snapshot name (dataset@short).
func (c *Client) DestroySnapshot(ctx context.Context, fullname string) (*Job, error) {
	if fullname == "" {
		return nil, errors.New("novanas: snapshot fullname is required")
	}
	resp, err := c.do(ctx, http.MethodDelete, "/snapshots/"+url.PathEscape(fullname), nil, nil, nil)
	if err != nil {
		return nil, err
	}
	return finishJobFromAccepted(resp)
}

// ---- Jobs -------------------------------------------------------------------

// GetJob fetches a single job by ID.
func (c *Client) GetJob(ctx context.Context, id string) (*Job, error) {
	if id == "" {
		return nil, errors.New("novanas: job id is required")
	}
	var out Job
	if _, err := c.do(ctx, http.MethodGet, "/jobs/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DefaultPollInterval is used by WaitJob when pollInterval is zero.
const DefaultPollInterval = 1 * time.Second

// WaitJob polls GetJob until the job reaches a terminal state or ctx is
// cancelled. If the job ends in a non-success terminal state, WaitJob
// returns the final Job along with a *JobFailedError.
//
// pollInterval <= 0 uses DefaultPollInterval.
func (c *Client) WaitJob(ctx context.Context, id string, pollInterval time.Duration) (*Job, error) {
	if pollInterval <= 0 {
		pollInterval = DefaultPollInterval
	}
	t := time.NewTimer(0) // fire immediately on first iteration
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-t.C:
		}
		job, err := c.GetJob(ctx, id)
		if err != nil {
			return nil, err
		}
		if job.IsTerminal() {
			if !job.Succeeded() {
				return job, &JobFailedError{Job: job}
			}
			return job, nil
		}
		t.Reset(pollInterval)
	}
}

