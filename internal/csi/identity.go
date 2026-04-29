package csi

import (
	"context"

	csipb "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// IdentityService implements csi.IdentityServer.
type IdentityService struct {
	csipb.UnimplementedIdentityServer
	d *Driver
}

// GetPluginInfo returns the driver name and version.
func (s *IdentityService) GetPluginInfo(ctx context.Context, _ *csipb.GetPluginInfoRequest) (*csipb.GetPluginInfoResponse, error) {
	return &csipb.GetPluginInfoResponse{
		Name:          s.d.cfg.Name,
		VendorVersion: s.d.cfg.Version,
	}, nil
}

// GetPluginCapabilities advertises CONTROLLER_SERVICE.
// (VOLUME_ACCESSIBILITY_CONSTRAINTS is omitted for v1: a single-node k3s
// deployment has no topology to express.)
func (s *IdentityService) GetPluginCapabilities(ctx context.Context, _ *csipb.GetPluginCapabilitiesRequest) (*csipb.GetPluginCapabilitiesResponse, error) {
	return &csipb.GetPluginCapabilitiesResponse{
		Capabilities: []*csipb.PluginCapability{
			{
				Type: &csipb.PluginCapability_Service_{
					Service: &csipb.PluginCapability_Service{
						Type: csipb.PluginCapability_Service_CONTROLLER_SERVICE,
					},
				},
			},
		},
	}, nil
}

// Probe returns ready=true if the NovaNAS API is reachable. We probe by
// querying the default pool's dataset; a NotFound counts as healthy because
// it proves we reached the API.
func (s *IdentityService) Probe(ctx context.Context, _ *csipb.ProbeRequest) (*csipb.ProbeResponse, error) {
	pool := s.d.cfg.DefaultPool
	if pool == "" {
		// No pool configured to probe against — declare ready and let
		// CreateVolume fail with a clear error if misconfigured.
		return &csipb.ProbeResponse{Ready: wrapperspb.Bool(true)}, nil
	}
	_, err := s.d.client.GetDataset(ctx, pool)
	if err != nil && !s.d.client.IsNotFound(err) {
		return &csipb.ProbeResponse{Ready: wrapperspb.Bool(false)}, nil
	}
	return &csipb.ProbeResponse{Ready: wrapperspb.Bool(true)}, nil
}
