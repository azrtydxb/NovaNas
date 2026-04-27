package novanas

import (
	"context"
	"net/http"
)

// ListBackendAssignments returns every BackendAssignment in the system.
// The endpoint is cluster-scoped and does not yet expose server-side
// label filtering — the controller filters by `novanas.io/pool` /
// `novanas.io/node` locally.
func (c *Client) ListBackendAssignments(ctx context.Context) ([]BackendAssignment, error) {
	return list[BackendAssignment](ctx, c, "/api/v1/backend-assignments")
}

// GetBackendAssignment returns a single BackendAssignment, or (nil, nil)
// when the API replies 404. Other errors surface as APIError.
func (c *Client) GetBackendAssignment(ctx context.Context, name string) (*BackendAssignment, error) {
	var ba BackendAssignment
	err := c.do(ctx, http.MethodGet, "/api/v1/backend-assignments/"+name, nil, &ba)
	if err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.Status == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &ba, nil
}

// CreateBackendAssignment posts a new BackendAssignment with the given
// metadata.name and spec. The status block is ignored on create — the
// agent populates it via PatchBackendAssignmentStatus.
func (c *Client) CreateBackendAssignment(ctx context.Context, ba *BackendAssignment) (*BackendAssignment, error) {
	if ba.APIVersion == "" {
		ba.APIVersion = "novanas.io/v1alpha1"
	}
	if ba.Kind == "" {
		ba.Kind = "BackendAssignment"
	}
	var out BackendAssignment
	if err := c.do(ctx, http.MethodPost, "/api/v1/backend-assignments", ba, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PatchBackendAssignmentSpec updates the spec block in-place using the
// API's JSON-merge semantics (top-level keys replace wholesale).
func (c *Client) PatchBackendAssignmentSpec(ctx context.Context, name string, spec BackendAssignmentSpec) error {
	return c.do(ctx, http.MethodPatch, "/api/v1/backend-assignments/"+name,
		map[string]any{"spec": spec}, nil)
}

// PatchBackendAssignmentStatus is the equivalent of the legacy
// r.Status().Update — write only the status fields.
func (c *Client) PatchBackendAssignmentStatus(ctx context.Context, name string, status BackendAssignmentStatus) error {
	return c.patchStatus(ctx, "/api/v1/backend-assignments/"+name, status)
}

// DeleteBackendAssignment removes an assignment. 404 is swallowed —
// orphan cleanup loops should treat already-gone as success.
func (c *Client) DeleteBackendAssignment(ctx context.Context, name string) error {
	err := c.do(ctx, http.MethodDelete, "/api/v1/backend-assignments/"+name, nil, nil)
	if err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.Status == http.StatusNotFound {
			return nil
		}
		return err
	}
	return nil
}
