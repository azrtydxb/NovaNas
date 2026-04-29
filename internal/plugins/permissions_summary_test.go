package plugins

import (
	"strings"
	"testing"
)

func TestSummarize_FromGoodManifest(t *testing.T) {
	p, err := ParseManifest([]byte(goodManifest))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	s := Summarize(p)
	if s.Category != "storage" {
		t.Errorf("category=%q", s.Category)
	}
	if len(s.WillCreate) != 4 {
		t.Fatalf("willCreate=%d, want 4", len(s.WillCreate))
	}
	// Ordering must match manifest order.
	wantKinds := []string{"dataset", "oidcClient", "tlsCert", "permission"}
	for i, want := range wantKinds {
		if s.WillCreate[i].Kind != want {
			t.Errorf("WillCreate[%d].Kind=%q want %q", i, s.WillCreate[i].Kind, want)
		}
		if s.WillCreate[i].Destructive {
			t.Errorf("WillCreate[%d].Destructive=true; v1 needs are non-destructive", i)
		}
	}
	if !strings.Contains(s.WillCreate[0].What, "tank/rustfs/data") {
		t.Errorf("dataset what=%q", s.WillCreate[0].What)
	}
	if !strings.Contains(s.WillCreate[1].What, "rustfs") {
		t.Errorf("oidc what=%q", s.WillCreate[1].What)
	}
	if !strings.Contains(s.WillCreate[2].What, "rustfs.local") {
		t.Errorf("tls what=%q", s.WillCreate[2].What)
	}
	if !strings.Contains(s.WillCreate[3].What, "rustfs-admin") {
		t.Errorf("perm what=%q", s.WillCreate[3].What)
	}
	if len(s.WillMount) != 1 {
		t.Fatalf("willMount=%v", s.WillMount)
	}
	wantMount := "/api/v1/plugins/rustfs/buckets/*"
	if s.WillMount[0] != wantMount {
		t.Errorf("willMount[0]=%q want %q", s.WillMount[0], wantMount)
	}
	if len(s.WillOpen) != 0 {
		t.Errorf("willOpen=%v; v1 must be empty", s.WillOpen)
	}
	if len(s.Scopes) != 1 || s.Scopes[0] != "PermPluginsRead" {
		t.Errorf("scopes=%v", s.Scopes)
	}
}

func TestSummarize_NilSafe(t *testing.T) {
	s := Summarize(nil)
	if s.WillCreate == nil || s.WillMount == nil || s.WillOpen == nil {
		t.Errorf("Summarize(nil) returned nil slices: %+v", s)
	}
	if len(s.WillCreate) != 0 || len(s.WillMount) != 0 || len(s.WillOpen) != 0 {
		t.Errorf("Summarize(nil) non-empty: %+v", s)
	}
}

func TestSummarize_EmptyNeedFields(t *testing.T) {
	p := &Plugin{
		Metadata: PluginMetadata{Name: "x"},
		Spec: PluginSpec{
			Category: CategoryUtility,
			Needs: []Need{
				{Kind: NeedDataset},    // Dataset nil — should still produce a generic entry
				{Kind: NeedOIDCClient}, // ditto
				{Kind: NeedTLSCert},
				{Kind: NeedPermission},
			},
		},
	}
	s := Summarize(p)
	if len(s.WillCreate) != 4 {
		t.Fatalf("willCreate=%d", len(s.WillCreate))
	}
	for i, e := range s.WillCreate {
		if e.What == "" {
			t.Errorf("WillCreate[%d].What empty", i)
		}
	}
}
