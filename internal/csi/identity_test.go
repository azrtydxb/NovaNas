package csi

import (
	"context"
	"testing"

	csipb "github.com/container-storage-interface/spec/lib/go/csi"
)

func newTestDriver(client NovaNASClient, mounter Mounter) *Driver {
	return NewDriver(Config{
		Name:        "csi.novanas.io",
		Version:     "0.1.0",
		NodeID:      "test-node",
		DefaultPool: "tank",
	}, client, mounter)
}

func TestGetPluginInfo(t *testing.T) {
	d := newTestDriver(newFakeClient(), newFakeMounter())
	resp, err := (&IdentityService{d: d}).GetPluginInfo(context.Background(), &csipb.GetPluginInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Name != "csi.novanas.io" || resp.VendorVersion != "0.1.0" {
		t.Fatalf("unexpected: %+v", resp)
	}
}

func TestGetPluginCapabilities(t *testing.T) {
	d := newTestDriver(newFakeClient(), newFakeMounter())
	resp, err := (&IdentityService{d: d}).GetPluginCapabilities(context.Background(), &csipb.GetPluginCapabilitiesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Capabilities) != 1 {
		t.Fatalf("expected 1 capability, got %d", len(resp.Capabilities))
	}
	svc := resp.Capabilities[0].GetService()
	if svc == nil || svc.Type != csipb.PluginCapability_Service_CONTROLLER_SERVICE {
		t.Fatalf("expected CONTROLLER_SERVICE, got %+v", svc)
	}
}

func TestProbe_OK_NotFoundIsReady(t *testing.T) {
	c := newFakeClient()
	d := newTestDriver(c, newFakeMounter())
	resp, err := (&IdentityService{d: d}).Probe(context.Background(), &csipb.ProbeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Ready.GetValue() {
		t.Fatalf("expected ready=true (NotFound is OK)")
	}
}
