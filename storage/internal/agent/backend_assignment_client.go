// Package agent: HTTP client for the API-managed BackendAssignment resource.
//
// Replaces the previous controller-runtime CRD client now that all NovaNas
// resources live in Postgres behind the API server (#70). The agent polls
// /api/v1/backend-assignments instead of watching a CRD.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// BackendAssignment mirrors the API JSON wire shape
// (packages/schemas/src/storage/backend-assignment.ts). Field names use
// the API's preferredClass / minSize convention rather than the storage
// CRD's Type / MinSize so the controller and agent agree on a single
// shape end-to-end.
type BackendAssignment struct {
	APIVersion string                  `json:"apiVersion"`
	Kind       string                  `json:"kind"`
	Metadata   ObjectMeta              `json:"metadata"`
	Spec       BackendAssignmentSpec   `json:"spec"`
	Status     BackendAssignmentStatus `json:"status,omitempty"`
}

// ObjectMeta is the trimmed envelope the API returns/accepts. Other
// fields (resourceVersion, creationTimestamp) are echoed back but the
// agent never sets them on writes.
type ObjectMeta struct {
	Name              string            `json:"name"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	ResourceVersion   string            `json:"resourceVersion,omitempty"`
	CreationTimestamp string            `json:"creationTimestamp,omitempty"`
}

type BackendAssignmentSpec struct {
	PoolRef      string             `json:"poolRef"`
	NodeName     string             `json:"nodeName"`
	BackendType  string             `json:"backendType"`
	DeviceFilter *APIDeviceFilter   `json:"deviceFilter,omitempty"`
	FileBackend  *APIFileBackend    `json:"fileBackend,omitempty"`
}

// APIDeviceFilter uses the API's wire shape (preferredClass/minSize)
// rather than the storage CRD's Type/MinSize.
type APIDeviceFilter struct {
	PreferredClass string `json:"preferredClass,omitempty"` // nvme | ssd | hdd
	MinSize        string `json:"minSize,omitempty"`        // e.g. "100Gi"
	MaxSize        string `json:"maxSize,omitempty"`
}

type APIFileBackend struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
}

type BackendAssignmentStatus struct {
	Phase      string                  `json:"phase,omitempty"`
	Device     string                  `json:"device,omitempty"`
	PCIeAddr   string                  `json:"pcieAddr,omitempty"`
	BdevName   string                  `json:"bdevName,omitempty"`
	Capacity   int64                   `json:"capacity,omitempty"`
	Message    string                  `json:"message,omitempty"`
	Conditions []map[string]any        `json:"conditions,omitempty"`
}

type backendAssignmentList struct {
	Items []BackendAssignment `json:"items"`
}

// BAClient is a thin HTTP client for /api/v1/backend-assignments.
type BAClient struct {
	BaseURL string // e.g. http://novanas-api.novanas-system.svc:8080
	HTTP    *http.Client
}

// NewBAClient builds a default-configured client.
func NewBAClient(baseURL string) *BAClient {
	return &BAClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}
}

// List returns all BackendAssignments. Server-side filtering by node is
// not exposed yet, so the agent filters on spec.NodeName locally.
func (c *BAClient) List(ctx context.Context) ([]BackendAssignment, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.BaseURL+"/api/v1/backend-assignments", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, c.errFromResp(resp, "list backend-assignments")
	}
	var out backendAssignmentList
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode list: %w", err)
	}
	return out.Items, nil
}

// Get returns a single BackendAssignment by name, or (nil, nil) on 404.
func (c *BAClient) Get(ctx context.Context, name string) (*BackendAssignment, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.BaseURL+"/api/v1/backend-assignments/"+name, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, c.errFromResp(resp, "get backend-assignment")
	}
	var out BackendAssignment
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode get: %w", err)
	}
	return &out, nil
}

// PatchStatus PATCHes only the status block. The API uses JSON-merge
// semantics on top-level keys; passing { "status": {...} } leaves
// metadata/labels/spec untouched.
func (c *BAClient) PatchStatus(ctx context.Context, name string, status BackendAssignmentStatus) (*BackendAssignment, error) {
	body, err := json.Marshal(map[string]any{"status": status})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch,
		c.BaseURL+"/api/v1/backend-assignments/"+name, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, c.errFromResp(resp, "patch status backend-assignment")
	}
	var out BackendAssignment
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode patch: %w", err)
	}
	return &out, nil
}

func (c *BAClient) errFromResp(resp *http.Response, op string) error {
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("%s: HTTP %d: %s", op, resp.StatusCode, strings.TrimSpace(string(body)))
}
