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

const manifestWithDeps = `apiVersion: novanas.io/v1
kind: Plugin
metadata:
  name: app
  version: 1.0.0
  vendor: ACME
spec:
  description: depends on object-storage
  category: utility
  deployment:
    type: systemd
    unit: app.service
  dependencies:
    - name: object-storage
      versionConstraint: ">=1.0.0,<2.0.0"
      source: tier-2
    - name: zfs-replication
      source: bundled
`

func TestParseManifest_GoodDependencies(t *testing.T) {
	p, err := ParseManifest([]byte(manifestWithDeps))
	if err != nil {
		t.Fatalf("expected good manifest, got %v", err)
	}
	if len(p.Spec.Dependencies) != 2 {
		t.Fatalf("expected 2 deps, got %d", len(p.Spec.Dependencies))
	}
	if p.Spec.Dependencies[0].Source != DependencySourceTier2 {
		t.Errorf("dep[0].source=%q", p.Spec.Dependencies[0].Source)
	}
	if p.Spec.Dependencies[1].Source != DependencySourceBundled {
		t.Errorf("dep[1].source=%q", p.Spec.Dependencies[1].Source)
	}
}

func TestParseManifest_BadDependencyName(t *testing.T) {
	bad := strings.Replace(manifestWithDeps, "name: object-storage", "name: BadName!", 1)
	if _, err := ParseManifest([]byte(bad)); err == nil {
		t.Fatal("expected bad dep name rejection")
	}
}

func TestParseManifest_BadDependencySource(t *testing.T) {
	bad := strings.Replace(manifestWithDeps, "source: tier-2", "source: rogue", 1)
	if _, err := ParseManifest([]byte(bad)); err == nil {
		t.Fatal("expected bad source rejection")
	}
}

func TestParseManifest_BadDependencyConstraint(t *testing.T) {
	bad := strings.Replace(manifestWithDeps, `versionConstraint: ">=1.0.0,<2.0.0"`, `versionConstraint: "garbage"`, 1)
	if _, err := ParseManifest([]byte(bad)); err == nil {
		t.Fatal("expected bad constraint rejection")
	}
}

func TestParseManifest_SelfDependency(t *testing.T) {
	bad := strings.Replace(manifestWithDeps, "name: object-storage", "name: app", 1)
	if _, err := ParseManifest([]byte(bad)); err == nil {
		t.Fatal("expected self-dep rejection")
	}
}

// --- displayCategory + tags ----------------------------------------------

func TestParseManifest_DisplayCategoryDefaultFromPrivilege(t *testing.T) {
	// goodManifest has category=storage and no displayCategory; the
	// fill-in helper must set displayCategory=storage.
	p, err := ParseManifest([]byte(goodManifest))
	if err != nil {
		t.Fatalf("expected good manifest, got %v", err)
	}
	if p.Spec.DisplayCategory != DisplayStorage {
		t.Errorf("displayCategory: want %q, got %q", DisplayStorage, p.Spec.DisplayCategory)
	}
}

func TestParseManifest_DisplayCategoryExplicit(t *testing.T) {
	manifest := strings.Replace(goodManifest,
		"  category: storage\n",
		"  category: storage\n  displayCategory: backup\n  tags: [\"s3\", \"backup-target\"]\n",
		1)
	p, err := ParseManifest([]byte(manifest))
	if err != nil {
		t.Fatalf("expected good manifest, got %v", err)
	}
	if p.Spec.DisplayCategory != DisplayBackup {
		t.Errorf("displayCategory=%q", p.Spec.DisplayCategory)
	}
	if len(p.Spec.Tags) != 2 || p.Spec.Tags[0] != "s3" {
		t.Errorf("tags=%+v", p.Spec.Tags)
	}
}

func TestParseManifest_DisplayCategoryUnknown(t *testing.T) {
	manifest := strings.Replace(goodManifest,
		"  category: storage\n",
		"  category: storage\n  displayCategory: bogus\n",
		1)
	if _, err := ParseManifest([]byte(manifest)); err == nil {
		t.Fatal("expected unknown displayCategory rejection")
	} else if !strings.Contains(err.Error(), "displayCategory") {
		t.Errorf("error should mention displayCategory, got %v", err)
	}
}

func TestParseManifest_TagBadChars(t *testing.T) {
	manifest := strings.Replace(goodManifest,
		"  category: storage\n",
		"  category: storage\n  tags: [\"BadTag\"]\n",
		1)
	if _, err := ParseManifest([]byte(manifest)); err == nil {
		t.Fatal("expected bad tag rejection")
	}
}

func TestParseManifest_TagTooLong(t *testing.T) {
	long := strings.Repeat("a", MaxTagLength+1)
	manifest := strings.Replace(goodManifest,
		"  category: storage\n",
		"  category: storage\n  tags: [\""+long+"\"]\n",
		1)
	if _, err := ParseManifest([]byte(manifest)); err == nil {
		t.Fatal("expected tag-length rejection")
	}
}

func TestParseManifest_TooManyTags(t *testing.T) {
	tags := make([]string, MaxTags+1)
	for i := range tags {
		tags[i] = "\"t" + string(rune('a'+i%26)) + "\""
	}
	manifest := strings.Replace(goodManifest,
		"  category: storage\n",
		"  category: storage\n  tags: ["+strings.Join(tags, ",")+"]\n",
		1)
	if _, err := ParseManifest([]byte(manifest)); err == nil {
		t.Fatal("expected too-many-tags rejection")
	}
}

func TestDefaultDisplayCategoryFor(t *testing.T) {
	cases := []struct {
		in   Category
		want DisplayCategory
	}{
		{CategoryStorage, DisplayStorage},
		{CategoryNetworking, DisplayNetwork},
		{CategoryObservability, DisplayObservability},
		{CategoryDeveloper, DisplayDeveloper},
		{CategoryUtility, DisplayUtilities},
		{Category("unknown"), DisplayCategory("")},
	}
	for _, tc := range cases {
		if got := DefaultDisplayCategoryFor(tc.in); got != tc.want {
			t.Errorf("DefaultDisplayCategoryFor(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}
