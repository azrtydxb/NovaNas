package plugins

import (
	"strings"
	"testing"
)

const goodManifest = `apiVersion: novanas.io/v1
kind: Plugin
metadata:
  name: rustfs
  version: 1.2.3
  vendor: NovaNAS Project
spec:
  description: S3-compatible object storage
  category: storage
  deployment:
    type: helm
    chart: chart/
    namespace: rustfs
  needs:
    - kind: dataset
      dataset:
        pool: tank
        name: rustfs/data
    - kind: oidcClient
      oidcClient:
        clientId: rustfs
    - kind: tlsCert
      tlsCert:
        commonName: rustfs.local
    - kind: permission
      permission:
        role: rustfs-admin
  api:
    routes:
      - path: /buckets
        upstream: http://127.0.0.1:9000
        auth: bearer-passthrough
  ui:
    window:
      name: RustFS
      route: /apps/rustfs
      bundle: main.js
`

func TestParseManifest_Good(t *testing.T) {
	p, err := ParseManifest([]byte(goodManifest))
	if err != nil {
		t.Fatalf("expected good manifest, got %v", err)
	}
	if p.Metadata.Name != "rustfs" {
		t.Errorf("name=%q", p.Metadata.Name)
	}
	if p.Spec.Deployment.Type != DeploymentHelm {
		t.Errorf("deployment=%q", p.Spec.Deployment.Type)
	}
	if len(p.Spec.Needs) != 4 {
		t.Errorf("needs=%d", len(p.Spec.Needs))
	}
}

func TestParseManifest_BadAPIVersion(t *testing.T) {
	bad := strings.Replace(goodManifest, "apiVersion: novanas.io/v1", "apiVersion: novanas.io/v0", 1)
	if _, err := ParseManifest([]byte(bad)); err == nil {
		t.Fatal("expected apiVersion error")
	}
}

func TestParseManifest_BadName(t *testing.T) {
	bad := strings.Replace(goodManifest, "name: rustfs", "name: BadName!", 1)
	if _, err := ParseManifest([]byte(bad)); err == nil {
		t.Fatal("expected name error")
	}
}

func TestParseManifest_BadVersion(t *testing.T) {
	bad := strings.Replace(goodManifest, "version: 1.2.3", "version: notsemver", 1)
	if _, err := ParseManifest([]byte(bad)); err == nil {
		t.Fatal("expected version error")
	}
}

func TestParseManifest_PrivilegeEscalation(t *testing.T) {
	// A category=utility plugin tries to claim a dataset — must be rejected.
	manifest := `apiVersion: novanas.io/v1
kind: Plugin
metadata:
  name: utilplug
  version: 0.1.0
  vendor: ACME
spec:
  description: tool
  category: utility
  deployment:
    type: systemd
    unit: utilplug.service
  needs:
    - kind: dataset
      dataset:
        pool: tank
        name: util/data
`
	if _, err := ParseManifest([]byte(manifest)); err == nil {
		t.Fatal("expected privilege-escalation rejection")
	} else if !strings.Contains(err.Error(), "may not claim") {
		t.Errorf("error should mention privilege rejection, got %v", err)
	}
}

func TestParseManifest_BadAuthMode(t *testing.T) {
	bad := strings.Replace(goodManifest, "auth: bearer-passthrough", "auth: open-bar", 1)
	if _, err := ParseManifest([]byte(bad)); err == nil {
		t.Fatal("expected auth mode rejection")
	}
}

func TestParseManifest_BadDeploymentType(t *testing.T) {
	bad := strings.Replace(goodManifest, "type: helm", "type: docker", 1)
	if _, err := ParseManifest([]byte(bad)); err == nil {
		t.Fatal("expected deployment type rejection")
	}
}
