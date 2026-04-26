package novanas

import (
	"context"
	"fmt"
	"net/url"
)

// Vm carries only the fields the operator-side worker uses to drive
// the runtime adapter — schema additions on the API side don't break
// us because json.Unmarshal ignores unknowns.
type Vm struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   ObjectMeta `json:"metadata"`
	Spec       VmSpec     `json:"spec"`
	Status     VmStatus   `json:"status,omitempty"`
}

type VmSpec struct {
	PowerState string         `json:"powerState,omitempty"`
	Spec       map[string]any `json:"spec,omitempty"`
}

type VmStatus struct {
	Phase      string      `json:"phase,omitempty"`
	Conditions []Condition `json:"conditions,omitempty"`
}

func (c *Client) ListVms(ctx context.Context, namespace string) ([]Vm, error) {
	if namespace == "" {
		return list[Vm](ctx, c, "/api/v1/vms")
	}
	return list[Vm](ctx, c, "/api/v1/vms?namespace="+url.QueryEscape(namespace))
}

func (c *Client) GetVm(ctx context.Context, namespace, name string) (*Vm, error) {
	var v Vm
	path := fmt.Sprintf("/api/v1/vms/%s/%s", url.PathEscape(namespace), url.PathEscape(name))
	if err := c.do(ctx, "GET", path, nil, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (c *Client) PatchVmStatus(ctx context.Context, namespace, name string, status VmStatus) error {
	path := fmt.Sprintf("/api/v1/vms/%s/%s", url.PathEscape(namespace), url.PathEscape(name))
	return c.patchStatus(ctx, path, status)
}
