package novanas

import "context"

// ListPools returns all StoragePool resources visible to the caller.
// Pools are cluster-scoped (non-namespaced), so the path doesn't
// carry a namespace.
func (c *Client) ListPools(ctx context.Context) ([]Pool, error) {
	return list[Pool](ctx, c, "/api/v1/pools")
}

// GetPool fetches a single StoragePool by name.
func (c *Client) GetPool(ctx context.Context, name string) (*Pool, error) {
	var p Pool
	if err := c.do(ctx, "GET", "/api/v1/pools/"+name, nil, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// PatchPoolStatus updates only the status block on a StoragePool. This
// is the equivalent of the controller-runtime r.Status().Update(...)
// path used by the previous CRD-based reconciler.
func (c *Client) PatchPoolStatus(ctx context.Context, name string, status PoolStatus) error {
	return c.patchStatus(ctx, "/api/v1/pools/"+name, status)
}

// ListDisks returns all Disk resources.
func (c *Client) ListDisks(ctx context.Context) ([]Disk, error) {
	return list[Disk](ctx, c, "/api/v1/disks")
}
