package v1alpha1

import (
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestAppSpecJSONRoundtrip ensures the wire tags on the apps/VM/sharing/system
// types the operator owns match the TypeScript schemas in packages/schemas.
// When the two drift (e.g., someone renames a field in one place but not the
// other) this test catches it by decoding a spec shaped like the Zod schema
// and re-marshalling to confirm key preservation.
func TestAppSpecJSONRoundtrip(t *testing.T) {
	cases := []struct {
		name string
		in   string
		out  any
	}{
		{
			name: "app",
			in: `{"displayName":"nextcloud","version":"29.0.0","chart":{"ociRef":"oci://registry/app/nextcloud","version":"29.0.0"},"requirements":{"minRamMB":512,"ports":[8080]}}`,
			out:  &AppSpec{},
		},
		{
			name: "appcatalog",
			in:   `{"source":{"type":"oci","url":"oci://registry/nvn/catalog"},"trust":{"signedBy":"ops","required":true}}`,
			out:  &AppCatalogSpec{},
		},
		{
			name: "appinstance",
			in:   `{"app":"nextcloud","version":"29.0.0","storage":[{"name":"data","dataset":"tank/nc","mode":"ReadWrite","mountPath":"/var/www/html/data"}],"network":{"expose":[{"port":8080,"protocol":"TCP","advertise":"lan"}]},"updates":{"autoUpdate":true,"channel":"stable"}}`,
			out:  &AppInstanceSpec{},
		},
		{
			name: "isolibrary",
			in:   `{"dataset":"tank/iso","sources":[{"url":"https://cdn.example/ubuntu-24.04.iso","sha256":"deadbeef"}]}`,
			out:  &IsoLibrarySpec{},
		},
		{
			name: "vm",
			in:   `{"os":{"type":"linux","variant":"debian12"},"resources":{"cpu":4,"memoryMiB":4096},"disks":[{"name":"root","source":{"type":"dataset","dataset":"tank/vm/root"},"bus":"virtio","boot":1}],"network":[{"type":"bridge","bridge":"br0","model":"virtio"}],"powerState":"Running"}`,
			out:  &VmSpec{},
		},
		{
			name: "gpudevice",
			in:   `{"passthrough":true}`,
			out:  &GpuDeviceSpec{},
		},
		{
			name: "upspolicy",
			in:   `{"integration":"nut","host":"127.0.0.1","port":3493,"thresholds":{"batteryPercent":30,"runtimeSeconds":300},"onBattery":["alertOnly"],"onLowBattery":["stopVms","shutdown"]}`,
			out:  &UpsPolicySpec{},
		},
		{
			name: "configbackuppolicy",
			in:   `{"cron":"0 3 * * *","destinations":[{"name":"local","type":"localPath","path":"/var/backups"}],"include":{"crds":true,"keycloak":true},"retention":{"keepLast":14}}`,
			out:  &ConfigBackupPolicySpec{},
		},
		{
			name: "updatepolicy",
			in:   `{"channel":"stable","autoUpdate":true,"autoReboot":false,"maintenanceWindow":{"cron":"0 3 * * 0","durationMinutes":60},"skipVersions":["1.2.3"]}`,
			out:  &UpdatePolicySpec{},
		},
		{
			name: "smartpolicy",
			in:   `{"appliesTo":{"all":true},"shortTest":{"cron":"0 0 * * *"},"longTest":{"cron":"0 0 * * 0"},"thresholds":{"temperature":{"warning":"50","critical":"60"}},"actions":{"onWarning":"alert","onCritical":"alertAndMarkDegraded"}}`,
			out:  &SmartPolicySpec{},
		},
		{
			name: "bucketuser",
			in:   `{"displayName":"backup-writer","credentials":{"accessKeySecret":{"secretName":"bu-ak"},"secretKeySecret":{"secretName":"bu-sk"}},"policies":[{"bucket":"backups","actions":["read","write","list"],"effect":"allow"}]}`,
			out:  &BucketUserSpec{},
		},
		{
			name: "objectstore",
			in:   `{"port":9000,"tls":{"enabled":true,"certificate":"s3-cert"},"region":"us-east-1","features":{"versioning":true,"objectLock":true}}`,
			out:  &ObjectStoreSpec{},
		},
		{
			name: "systemsettings",
			in:   `{"hostname":"nas01","timezone":"Europe/Brussels","locale":"en_US.UTF-8","ntp":{"servers":["time.cloudflare.com"],"enabled":true},"smtp":{"host":"smtp.example","port":587,"encryption":"starttls","from":"ops@example","authSecret":{"secretName":"smtp-auth"}}}`,
			out:  &SystemSettingsSpec{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(tc.in), tc.out); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			out, err := json.Marshal(tc.out)
			if err != nil {
				t.Fatalf("remarshal: %v", err)
			}
			var reparsed map[string]any
			if err := json.Unmarshal(out, &reparsed); err != nil {
				t.Fatalf("reparse: %v\nraw: %s", err, out)
			}
			var original map[string]any
			if err := json.Unmarshal([]byte(tc.in), &original); err != nil {
				t.Fatalf("original parse: %v", err)
			}
			// Every non-zero key in the original input must survive the
			// roundtrip. Zero-valued primitives legitimately drop because
			// of omitempty tags and are therefore ignored.
			for k, v := range original {
				if isZeroJSON(v) {
					continue
				}
				if _, ok := reparsed[k]; !ok {
					t.Errorf("key %q lost in roundtrip\noriginal: %s\nroundtrip: %s", k, tc.in, out)
				}
			}
		})
	}
}

// isZeroJSON reports whether a JSON value decoded via encoding/json would be
// elided by encoding/json's omitempty tag.
func isZeroJSON(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case bool:
		return !x
	case string:
		return x == ""
	case float64:
		return x == 0
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	}
	return false
}

// TestDeepCopyObject spot-checks DeepCopyObject for the richer specs to
// guarantee controller-gen emitted a working copy (slice/pointer fields).
func TestDeepCopyObject(t *testing.T) {
	vm := &Vm{
		ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: "default"},
		Spec: VmSpec{
			OS:        VmOS{Type: "linux"},
			Resources: VmResources{CPU: 2, MemoryMiB: 2048},
			Disks: []VmDisk{
				{Name: "root", Source: VmDiskSource{Type: "dataset", Dataset: "tank/root"}, Bus: "virtio"},
			},
			Network: []VmNetwork{{Type: "bridge", Bridge: "br0"}},
		},
		Status: VmStatus{
			Phase: "Running",
			Conditions: []metav1.Condition{{
				Type:    "Ready",
				Status:  metav1.ConditionTrue,
				Reason:  "Ok",
				Message: "running",
			}},
		},
	}
	cp := vm.DeepCopy()
	if cp == vm {
		t.Fatal("DeepCopy returned same pointer")
	}
	cp.Spec.Disks[0].Name = "mutated"
	if vm.Spec.Disks[0].Name == "mutated" {
		t.Error("DeepCopy shared Disks slice backing array")
	}
	cp.Status.Conditions[0].Message = "changed"
	if vm.Status.Conditions[0].Message == "changed" {
		t.Error("DeepCopy shared Conditions slice backing array")
	}

	ai := &AppInstance{Spec: AppInstanceSpec{App: "nc", Storage: []AppInstanceStorage{{Name: "data"}}}}
	ai2 := ai.DeepCopy()
	ai2.Spec.Storage[0].Name = "altered"
	if ai.Spec.Storage[0].Name == "altered" {
		t.Error("AppInstance DeepCopy shared Storage slice")
	}
}
