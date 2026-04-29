package novanas

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// WorkloadIndexEntry mirrors workloads.IndexEntry on the server. It is
// the metadata for one curated app in the chart catalog.
type WorkloadIndexEntry struct {
	Name             string                 `json:"name"`
	DisplayName      string                 `json:"displayName,omitempty"`
	Category         string                 `json:"category,omitempty"`
	Description      string                 `json:"description,omitempty"`
	Chart            string                 `json:"chart"`
	Version          string                 `json:"version"`
	RepoURL          string                 `json:"repoURL"`
	AppVersion       string                 `json:"appVersion,omitempty"`
	Icon             string                 `json:"icon,omitempty"`
	Homepage         string                 `json:"homepage,omitempty"`
	ReadmeURL        string                 `json:"readmeURL,omitempty"`
	DefaultNamespace string                 `json:"defaultNamespace,omitempty"`
	Permissions      []string               `json:"permissions,omitempty"`
	DefaultValues    map[string]interface{} `json:"defaultValues,omitempty"`
}

// WorkloadIndexEntryDetail is the response of GET /workloads/index/{name}.
// Readme and ValuesSchema are best-effort: empty when nova-api could
// not fetch the chart from upstream.
type WorkloadIndexEntryDetail struct {
	WorkloadIndexEntry
	Readme       string                 `json:"readme,omitempty"`
	ValuesSchema map[string]interface{} `json:"valuesSchema,omitempty"`
}

// WorkloadRelease is a single installed app (Helm release).
type WorkloadRelease struct {
	Name        string    `json:"name"`
	Namespace   string    `json:"namespace"`
	IndexName   string    `json:"indexName,omitempty"`
	Chart       string    `json:"chart"`
	Version     string    `json:"version"`
	AppVersion  string    `json:"appVersion,omitempty"`
	Status      string    `json:"status"`
	Revision    int       `json:"revision"`
	Updated     time.Time `json:"updated"`
	InstalledBy string    `json:"installedBy,omitempty"`
	Notes       string    `json:"notes,omitempty"`
}

// WorkloadResourceRef mirrors the server's workloads.ResourceRef.
type WorkloadResourceRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace,omitempty"`
}

// WorkloadPodInfo mirrors the server's workloads.PodInfo.
type WorkloadPodInfo struct {
	Name       string   `json:"name"`
	Phase      string   `json:"phase"`
	Ready      bool     `json:"ready"`
	Restarts   int32    `json:"restarts"`
	Containers []string `json:"containers,omitempty"`
	NodeName   string   `json:"nodeName,omitempty"`
}

// WorkloadReleaseDetail is the GET /workloads/{name} response.
type WorkloadReleaseDetail struct {
	WorkloadRelease
	Values    map[string]interface{} `json:"values,omitempty"`
	Resources []WorkloadResourceRef  `json:"resources,omitempty"`
	Pods      []WorkloadPodInfo      `json:"pods,omitempty"`
}

// WorkloadEvent is one Kubernetes event in the release namespace.
type WorkloadEvent struct {
	Type      string    `json:"type"`
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	Object    string    `json:"object"`
	Count     int32     `json:"count"`
	FirstSeen time.Time `json:"firstSeen"`
	LastSeen  time.Time `json:"lastSeen"`
}

// WorkloadInstallRequest is the body of POST /workloads.
type WorkloadInstallRequest struct {
	IndexName   string `json:"indexName"`
	ReleaseName string `json:"releaseName"`
	ValuesYAML  string `json:"valuesYAML,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
}

// WorkloadUpgradeRequest is the body of PATCH /workloads/{name}.
type WorkloadUpgradeRequest struct {
	Version    string `json:"version,omitempty"`
	ValuesYAML string `json:"valuesYAML,omitempty"`
}

// WorkloadLogOptions controls the GET /workloads/{name}/logs query.
type WorkloadLogOptions struct {
	Pod          string
	Container    string
	Follow       bool
	TailLines    int64
	SinceSeconds int64
	Timestamps   bool
	Previous     bool
}

// ListWorkloadIndex returns the curated chart catalog.
func (c *Client) ListWorkloadIndex(ctx context.Context) ([]WorkloadIndexEntry, error) {
	var out []WorkloadIndexEntry
	if _, err := c.do(ctx, http.MethodGet, "/workloads/index", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetWorkloadIndexEntry returns one catalog entry plus README/schema.
func (c *Client) GetWorkloadIndexEntry(ctx context.Context, name string) (*WorkloadIndexEntryDetail, error) {
	if name == "" {
		return nil, errors.New("novanas: name is required")
	}
	var out WorkloadIndexEntryDetail
	if _, err := c.do(ctx, http.MethodGet, "/workloads/index/"+url.PathEscape(name), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ReloadWorkloadIndex re-reads the on-disk chart index.
func (c *Client) ReloadWorkloadIndex(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodPost, "/workloads/index/reload", nil, nil, nil)
	return err
}

// ListWorkloads returns all installed apps.
func (c *Client) ListWorkloads(ctx context.Context) ([]WorkloadRelease, error) {
	var out []WorkloadRelease
	if _, err := c.do(ctx, http.MethodGet, "/workloads", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetWorkload returns one installed app's detail.
func (c *Client) GetWorkload(ctx context.Context, releaseName string) (*WorkloadReleaseDetail, error) {
	if releaseName == "" {
		return nil, errors.New("novanas: releaseName is required")
	}
	var out WorkloadReleaseDetail
	if _, err := c.do(ctx, http.MethodGet, "/workloads/"+url.PathEscape(releaseName), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// InstallWorkload installs an app from the catalog.
func (c *Client) InstallWorkload(ctx context.Context, req WorkloadInstallRequest) (*WorkloadRelease, error) {
	if req.IndexName == "" || req.ReleaseName == "" {
		return nil, errors.New("novanas: WorkloadInstallRequest.IndexName and ReleaseName are required")
	}
	var out WorkloadRelease
	if _, err := c.do(ctx, http.MethodPost, "/workloads", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpgradeWorkload upgrades an existing release.
func (c *Client) UpgradeWorkload(ctx context.Context, releaseName string, req WorkloadUpgradeRequest) (*WorkloadRelease, error) {
	if releaseName == "" {
		return nil, errors.New("novanas: releaseName is required")
	}
	if req.Version == "" && req.ValuesYAML == "" {
		return nil, errors.New("novanas: UpgradeRequest must set Version, ValuesYAML, or both")
	}
	var out WorkloadRelease
	if _, err := c.do(ctx, http.MethodPatch, "/workloads/"+url.PathEscape(releaseName), nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UninstallWorkload removes the release and its namespace.
func (c *Client) UninstallWorkload(ctx context.Context, releaseName string) error {
	if releaseName == "" {
		return errors.New("novanas: releaseName is required")
	}
	_, err := c.do(ctx, http.MethodDelete, "/workloads/"+url.PathEscape(releaseName), nil, nil, nil)
	return err
}

// RollbackWorkload rolls a release back to revision.
func (c *Client) RollbackWorkload(ctx context.Context, releaseName string, revision int) (*WorkloadRelease, error) {
	if releaseName == "" {
		return nil, errors.New("novanas: releaseName is required")
	}
	body := map[string]int{"revision": revision}
	var out WorkloadRelease
	if _, err := c.do(ctx, http.MethodPost, "/workloads/"+url.PathEscape(releaseName)+"/rollback", nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// WorkloadEvents returns recent k8s events from the release namespace.
func (c *Client) WorkloadEvents(ctx context.Context, releaseName string) ([]WorkloadEvent, error) {
	if releaseName == "" {
		return nil, errors.New("novanas: releaseName is required")
	}
	var out []WorkloadEvent
	if _, err := c.do(ctx, http.MethodGet, "/workloads/"+url.PathEscape(releaseName)+"/events", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// WorkloadLogs streams logs from a pod in the release's namespace. The
// returned reader MUST be closed by the caller. When opts.Follow is
// true the reader stays open until ctx is cancelled.
func (c *Client) WorkloadLogs(ctx context.Context, releaseName string, opts WorkloadLogOptions) (io.ReadCloser, error) {
	if releaseName == "" {
		return nil, errors.New("novanas: releaseName is required")
	}
	q := url.Values{}
	if opts.Pod != "" {
		q.Set("pod", opts.Pod)
	}
	if opts.Container != "" {
		q.Set("container", opts.Container)
	}
	if opts.Follow {
		q.Set("follow", "true")
	}
	if opts.Timestamps {
		q.Set("timestamps", "true")
	}
	if opts.Previous {
		q.Set("previous", "true")
	}
	if opts.TailLines > 0 {
		q.Set("tail", strconv.FormatInt(opts.TailLines, 10))
	}
	if opts.SinceSeconds > 0 {
		q.Set("sinceSeconds", strconv.FormatInt(opts.SinceSeconds, 10))
	}
	u := c.BaseURL + apiPrefix + "/workloads/" + url.PathEscape(releaseName) + "/logs"
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/plain")
	if ua := c.UserAgent; ua != "" {
		req.Header.Set("User-Agent", ua)
	} else {
		req.Header.Set("User-Agent", DefaultUserAgent)
	}
	if tok := c.token(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		buf, _ := io.ReadAll(resp.Body)
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Code:       extractErrCode(buf),
			Message:    strings.TrimSpace(string(buf)),
		}
	}
	return resp.Body, nil
}

// extractErrCode pulls the "error" field out of a JSON error envelope
// best-effort, falling back to the HTTP status text.
func extractErrCode(buf []byte) string {
	s := string(buf)
	i := strings.Index(s, `"error"`)
	if i < 0 {
		return ""
	}
	rest := s[i+len(`"error"`):]
	if j := strings.Index(rest, `"`); j >= 0 {
		rest = rest[j+1:]
		if k := strings.Index(rest, `"`); k > 0 {
			return rest[:k]
		}
	}
	return ""
}

// avoid unused-import errors when tests are absent
var _ = fmt.Sprint
