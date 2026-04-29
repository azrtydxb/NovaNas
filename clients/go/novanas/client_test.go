package novanas

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// helper -----------------------------------------------------------------------

func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c, err := New(Config{BaseURL: srv.URL, Token: "tok-abc", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// httptest.NewServer is plain HTTP; replace transport so we don't fight
	// with TLS settings from New.
	c.HTTPClient = srv.Client()
	c.HTTPClient.Timeout = 5 * time.Second
	c.Token = "tok-abc"
	return c
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, body any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatalf("encode: %v", err)
	}
}

func assertAuth(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get("Authorization"); got != "Bearer tok-abc" {
		t.Errorf("Authorization header = %q, want %q", got, "Bearer tok-abc")
	}
	if !strings.HasPrefix(r.Header.Get("User-Agent"), "novanas-go/") {
		t.Errorf("User-Agent = %q, want novanas-go/* prefix", r.Header.Get("User-Agent"))
	}
}

// ---- Pools ------------------------------------------------------------------

func TestListPools(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAuth(t, r)
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/pools" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		writeJSON(t, w, 200, []Pool{{Name: "tank", SizeBytes: 1024, Health: "ONLINE"}})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	pools, err := c.ListPools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(pools) != 1 || pools[0].Name != "tank" || pools[0].Health != "ONLINE" {
		t.Errorf("unexpected pools: %+v", pools)
	}
}

func TestGetPool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/pools/tank" {
			t.Errorf("path %q", r.URL.Path)
		}
		writeJSON(t, w, 200, PoolDetail{
			Pool:       Pool{Name: "tank"},
			Properties: map[string]string{"compression": "lz4"},
			Status:     PoolStatus{State: "ONLINE"},
		})
	}))
	defer srv.Close()

	got, err := newTestClient(t, srv).GetPool(context.Background(), "tank")
	if err != nil {
		t.Fatal(err)
	}
	if got.Pool.Name != "tank" || got.Properties["compression"] != "lz4" || got.Status.State != "ONLINE" {
		t.Errorf("got %+v", got)
	}
}

// ---- Datasets ---------------------------------------------------------------

func TestListDatasets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/datasets" {
			t.Errorf("path %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("pool"); got != "tank" {
			t.Errorf("pool query = %q", got)
		}
		writeJSON(t, w, 200, []Dataset{{Name: "tank/data", Type: DatasetTypeFilesystem, UsedBytes: 42}})
	}))
	defer srv.Close()

	out, err := newTestClient(t, srv).ListDatasets(context.Background(), "tank")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Name != "tank/data" || out[0].UsedBytes != 42 {
		t.Errorf("unexpected: %+v", out)
	}
}

func TestGetDataset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// fullname has '/' which is escaped to %2F by url.PathEscape.
		if r.URL.EscapedPath() != "/api/v1/datasets/tank%2Fdata" {
			t.Errorf("escaped path %q", r.URL.EscapedPath())
		}
		writeJSON(t, w, 200, DatasetDetail{
			Dataset:    Dataset{Name: "tank/data"},
			Properties: map[string]string{"recordsize": "16K"},
		})
	}))
	defer srv.Close()

	got, err := newTestClient(t, srv).GetDataset(context.Background(), "tank/data")
	if err != nil {
		t.Fatal(err)
	}
	if got.Dataset.Name != "tank/data" || got.Properties["recordsize"] != "16K" {
		t.Errorf("got %+v", got)
	}
}

func TestCreateDataset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/datasets" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type = %q", r.Header.Get("Content-Type"))
		}
		var body CreateDatasetSpec
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Parent != "tank" || body.Name != "vol1" || body.Type != DatasetTypeVolume || body.VolumeSizeBytes != 1<<30 {
			t.Errorf("body: %+v", body)
		}
		w.Header().Set("Location", "/api/v1/jobs/00000000-0000-0000-0000-000000000001")
		writeJSON(t, w, 202, map[string]string{"jobId": "00000000-0000-0000-0000-000000000001"})
	}))
	defer srv.Close()

	job, err := newTestClient(t, srv).CreateDataset(context.Background(), CreateDatasetSpec{
		Parent: "tank", Name: "vol1", Type: DatasetTypeVolume, VolumeSizeBytes: 1 << 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.ID != "00000000-0000-0000-0000-000000000001" || job.State != JobStateQueued {
		t.Errorf("job: %+v", job)
	}
}

func TestCreateDatasetValidation(t *testing.T) {
	c := &Client{BaseURL: "http://x", HTTPClient: http.DefaultClient}
	if _, err := c.CreateDataset(context.Background(), CreateDatasetSpec{Type: DatasetTypeVolume, Parent: "p", Name: "n"}); err == nil {
		t.Fatal("expected volume validation error")
	}
}

func TestDestroyDataset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method %s", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v1/datasets/tank%2Fdata" {
			t.Errorf("path %q", r.URL.EscapedPath())
		}
		if r.URL.Query().Get("recursive") != "true" {
			t.Errorf("recursive = %q", r.URL.Query().Get("recursive"))
		}
		w.Header().Set("Location", "/api/v1/jobs/job-1")
		writeJSON(t, w, 202, map[string]string{"jobId": "job-1"})
	}))
	defer srv.Close()

	job, err := newTestClient(t, srv).DestroyDataset(context.Background(), "tank/data", true)
	if err != nil {
		t.Fatal(err)
	}
	if job.ID != "job-1" {
		t.Errorf("job: %+v", job)
	}
}

func TestSetDatasetProps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method %s", r.Method)
		}
		var body struct {
			Properties map[string]string `json:"properties"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Properties["compression"] != "lz4" {
			t.Errorf("body %+v", body)
		}
		writeJSON(t, w, 202, map[string]string{"jobId": "j2"})
	}))
	defer srv.Close()

	job, err := newTestClient(t, srv).SetDatasetProps(context.Background(), "tank/data", map[string]string{"compression": "lz4"})
	if err != nil {
		t.Fatal(err)
	}
	if job.ID != "j2" {
		t.Errorf("id %q", job.ID)
	}
}

func TestRenameDataset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.EscapedPath(), "/rename") {
			t.Errorf("path %q", r.URL.EscapedPath())
		}
		var body struct {
			NewName string `json:"newName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.NewName != "tank/new" {
			t.Errorf("newName %q", body.NewName)
		}
		writeJSON(t, w, 202, map[string]string{"jobId": "j3"})
	}))
	defer srv.Close()

	job, err := newTestClient(t, srv).RenameDataset(context.Background(), "tank/old", "tank/new")
	if err != nil || job.ID != "j3" {
		t.Fatalf("err=%v job=%+v", err, job)
	}
}

func TestCloneSnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/api/v1/datasets/" + escape("tank/data@s1") + "/clone"
		if r.URL.EscapedPath() != want {
			t.Errorf("path %q want %q", r.URL.EscapedPath(), want)
		}
		var body struct {
			Target     string            `json:"target"`
			Properties map[string]string `json:"properties"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Target != "tank/clone1" || body.Properties["compression"] != "lz4" {
			t.Errorf("body %+v", body)
		}
		writeJSON(t, w, 202, map[string]string{"jobId": "j4"})
	}))
	defer srv.Close()

	job, err := newTestClient(t, srv).CloneSnapshot(context.Background(), "tank/data@s1", "tank/clone1", map[string]string{"compression": "lz4"})
	if err != nil || job.ID != "j4" {
		t.Fatalf("err=%v job=%+v", err, job)
	}
}

// escape is a thin alias around url.PathEscape so the tests document
// what they expect on the wire.
func escape(s string) string { return url.PathEscape(s) }

// ---- Snapshots --------------------------------------------------------------

func TestListSnapshots(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/snapshots" {
			t.Errorf("path %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("dataset"); got != "tank/data" {
			t.Errorf("dataset query %q", got)
		}
		writeJSON(t, w, 200, []Snapshot{{Name: "tank/data@s1", Dataset: "tank/data", ShortName: "s1"}})
	}))
	defer srv.Close()

	out, err := newTestClient(t, srv).ListSnapshots(context.Background(), "tank/data")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].ShortName != "s1" {
		t.Errorf("out %+v", out)
	}
}

func TestCreateSnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/snapshots" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Dataset   string `json:"dataset"`
			Name      string `json:"name"`
			Recursive bool   `json:"recursive"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Dataset != "tank/data" || body.Name != "s1" || !body.Recursive {
			t.Errorf("body %+v", body)
		}
		writeJSON(t, w, 202, map[string]string{"jobId": "snap-1"})
	}))
	defer srv.Close()

	job, err := newTestClient(t, srv).CreateSnapshot(context.Background(), "tank/data", "s1", true)
	if err != nil || job.ID != "snap-1" {
		t.Fatalf("err=%v job=%+v", err, job)
	}
}

func TestDestroySnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method %s", r.Method)
		}
		want := "/api/v1/snapshots/" + escape("tank/data@s1")
		if r.URL.EscapedPath() != want {
			t.Errorf("path %q want %q", r.URL.EscapedPath(), want)
		}
		writeJSON(t, w, 202, map[string]string{"jobId": "snap-d"})
	}))
	defer srv.Close()

	job, err := newTestClient(t, srv).DestroySnapshot(context.Background(), "tank/data@s1")
	if err != nil || job.ID != "snap-d" {
		t.Fatalf("err=%v job=%+v", err, job)
	}
}

// ---- Jobs / Wait ------------------------------------------------------------

func TestGetJob(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/jobs/abc" {
			t.Errorf("path %q", r.URL.Path)
		}
		writeJSON(t, w, 200, Job{ID: "abc", State: JobStateRunning})
	}))
	defer srv.Close()

	job, err := newTestClient(t, srv).GetJob(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if job.ID != "abc" || job.State != JobStateRunning {
		t.Errorf("job %+v", job)
	}
}

func TestWaitJob_RunningThenSucceeded(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		state := JobStateRunning
		if n >= 4 {
			state = JobStateSucceeded
		}
		writeJSON(t, w, 200, Job{ID: "j", State: state})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	job, err := c.WaitJob(context.Background(), "j", 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if job.State != JobStateSucceeded {
		t.Errorf("state %q", job.State)
	}
	if calls.Load() < 4 {
		t.Errorf("expected at least 4 polls, got %d", calls.Load())
	}
}

func TestWaitJob_Failed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errStr := "boom"
		writeJSON(t, w, 200, Job{ID: "j", State: JobStateFailed, Error: &errStr})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	job, err := c.WaitJob(context.Background(), "j", 5*time.Millisecond)
	if err == nil {
		t.Fatal("expected JobFailedError")
	}
	var jfe *JobFailedError
	if !errorAs(err, &jfe) {
		t.Fatalf("err %T = %v, want *JobFailedError", err, err)
	}
	if job == nil || job.State != JobStateFailed {
		t.Errorf("job %+v", job)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error message %q missing reason", err.Error())
	}
}

func TestWaitJob_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, 200, Job{ID: "j", State: JobStateRunning})
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := newTestClient(t, srv).WaitJob(ctx, "j", 10*time.Millisecond)
	if err == nil {
		t.Fatal("expected context error")
	}
}

// errorAs wraps errors.As without importing the package twice for tests
// that only need it once.
func errorAs(err error, target any) bool {
	type interfaceish interface{ Error() string }
	_ = interfaceish(nil)
	switch t := target.(type) {
	case **JobFailedError:
		if e, ok := err.(*JobFailedError); ok {
			*t = e
			return true
		}
	}
	return false
}

// ---- Errors -----------------------------------------------------------------

func TestNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, 404, map[string]string{"error": "not_found", "message": "no such pool"})
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv).GetPool(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsNotFound(err) {
		t.Errorf("IsNotFound = false for %v", err)
	}
	if IsConflict(err) || IsForbidden(err) {
		t.Errorf("misclassified")
	}
}

func TestConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, 409, map[string]string{"error": "duplicate", "message": "in flight"})
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv).CreateDataset(context.Background(), CreateDatasetSpec{
		Parent: "tank", Name: "x", Type: DatasetTypeFilesystem,
	})
	if !IsConflict(err) {
		t.Errorf("IsConflict false for %v", err)
	}
}

func TestForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, 403, map[string]string{"error": "forbidden", "message": "no perm"})
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv).ListPools(context.Background())
	if !IsForbidden(err) {
		t.Errorf("IsForbidden false for %v", err)
	}
}

func TestAPIErrorMessage(t *testing.T) {
	e := &APIError{StatusCode: 500, Code: "boom", Message: "kaboom"}
	if !strings.Contains(e.Error(), "500") || !strings.Contains(e.Error(), "boom") {
		t.Errorf("APIError.Error() = %q", e.Error())
	}
}

// ---- TLS --------------------------------------------------------------------

func TestNew_TLSWithCAPEM(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, 200, []Pool{{Name: "tank"}})
	}))
	defer srv.Close()

	// Encode the test server's cert to PEM and feed it to New as the CA.
	cert := srv.Certificate()
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	if pemBytes == nil {
		t.Fatal("failed to encode test cert to PEM")
	}

	// Sanity: the cert really is a parseable cert.
	if _, err := x509.ParseCertificate(cert.Raw); err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	c, err := New(Config{BaseURL: srv.URL, Token: "t", CACertPEM: pemBytes, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	pools, err := c.ListPools(context.Background())
	if err != nil {
		t.Fatalf("ListPools over TLS: %v", err)
	}
	if len(pools) != 1 || pools[0].Name != "tank" {
		t.Errorf("pools %+v", pools)
	}
}

func TestNew_TLSInsecureSkipVerify(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, 200, []Pool{})
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL, InsecureSkipVerify: true, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.ListPools(context.Background()); err != nil {
		t.Fatalf("ListPools: %v", err)
	}
}

func TestNew_BadCAPEM(t *testing.T) {
	if _, err := New(Config{BaseURL: "https://x", CACertPEM: []byte("not a pem")}); err == nil {
		t.Fatal("expected error for bogus CA PEM")
	}
}

func TestNew_RequiresBaseURL(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected error for missing BaseURL")
	}
}

// ---- SetToken --------------------------------------------------------------

func TestSetToken_RotatesAuthHeader(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		writeJSON(t, w, 200, []Pool{})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if _, err := c.ListPools(context.Background()); err != nil {
		t.Fatalf("first ListPools: %v", err)
	}

	c.SetToken("rotated-xyz")
	if got := c.token(); got != "rotated-xyz" {
		t.Errorf("token() = %q, want rotated-xyz", got)
	}
	if _, err := c.ListPools(context.Background()); err != nil {
		t.Fatalf("second ListPools: %v", err)
	}

	c.SetToken("")
	if _, err := c.ListPools(context.Background()); err != nil {
		t.Fatalf("third ListPools: %v", err)
	}

	if len(seen) != 3 {
		t.Fatalf("calls=%d want 3", len(seen))
	}
	if seen[0] != "Bearer tok-abc" {
		t.Errorf("call0 auth=%q", seen[0])
	}
	if seen[1] != "Bearer rotated-xyz" {
		t.Errorf("call1 auth=%q", seen[1])
	}
	if seen[2] != "" {
		t.Errorf("call2 auth=%q (expected empty after SetToken(\"\"))", seen[2])
	}
}

func TestSetToken_Concurrent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, 200, []Pool{})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		i := 0
		for {
			select {
			case <-stop:
				return
			default:
				c.SetToken("t-" + strconv.Itoa(i))
				i++
			}
		}
	}()
	for i := 0; i < 50; i++ {
		if _, err := c.ListPools(context.Background()); err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
	}
	close(stop)
	<-done
}

// silence unused-import warning in case io.Reader stops being referenced
// by future edits.
var _ io.Reader = (*strings.Reader)(nil)

// ---- ProtocolShares --------------------------------------------------------

func TestCreateProtocolShare(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAuth(t, r)
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/protocol-shares" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		var body ProtocolShare
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Name != "share1" || body.Pool != "tank" || body.DatasetName != "csi-nfs/share1" {
			t.Errorf("body %+v", body)
		}
		if len(body.Protocols) != 1 || body.Protocols[0] != ProtocolNFS {
			t.Errorf("protocols %+v", body.Protocols)
		}
		if body.NFS == nil || len(body.NFS.Clients) != 1 || body.NFS.Clients[0].Spec != "10.0.0.0/8" {
			t.Errorf("nfs opts %+v", body.NFS)
		}
		writeJSON(t, w, 202, map[string]string{"jobId": "ps-1"})
	}))
	defer srv.Close()

	share := ProtocolShare{
		Name:        "share1",
		Pool:        "tank",
		DatasetName: "csi-nfs/share1",
		Protocols:   []Protocol{ProtocolNFS},
		Acls:        []DatasetACE{},
		NFS: &ProtocolNFSOpts{
			Clients: []NfsClientRule{{Spec: "10.0.0.0/8", Options: "rw,sync"}},
		},
	}
	job, err := newTestClient(t, srv).CreateProtocolShare(context.Background(), share)
	if err != nil {
		t.Fatal(err)
	}
	if job.ID != "ps-1" || job.State != JobStateQueued {
		t.Errorf("job %+v", job)
	}
}

func TestCreateProtocolShareValidation(t *testing.T) {
	c := &Client{BaseURL: "http://x", HTTPClient: http.DefaultClient}
	if _, err := c.CreateProtocolShare(context.Background(), ProtocolShare{}); err == nil {
		t.Fatal("expected validation error for empty share")
	}
	if _, err := c.CreateProtocolShare(context.Background(), ProtocolShare{
		Name: "x", Pool: "tank", DatasetName: "d",
	}); err == nil {
		t.Fatal("expected validation error for empty protocols")
	}
}

func TestGetProtocolShare(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/protocol-shares/share1" {
			t.Errorf("path %q", r.URL.Path)
		}
		if r.URL.Query().Get("pool") != "tank" || r.URL.Query().Get("dataset") != "csi-nfs/share1" {
			t.Errorf("query %v", r.URL.Query())
		}
		writeJSON(t, w, 200, ProtocolShareDetail{
			Share: ProtocolShare{
				Name: "share1", Pool: "tank", DatasetName: "csi-nfs/share1",
				Protocols: []Protocol{ProtocolNFS},
			},
			Path: "/tank/csi-nfs/share1",
			ProtocolsStatus: []ProtocolStatus{
				{Protocol: ProtocolNFS, Active: true},
			},
		})
	}))
	defer srv.Close()

	got, err := newTestClient(t, srv).GetProtocolShare(context.Background(), "share1", "tank", "csi-nfs/share1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Share.Name != "share1" || got.Path != "/tank/csi-nfs/share1" {
		t.Errorf("got %+v", got)
	}
	if len(got.ProtocolsStatus) != 1 || !got.ProtocolsStatus[0].Active {
		t.Errorf("status %+v", got.ProtocolsStatus)
	}
}

func TestGetProtocolShareValidation(t *testing.T) {
	c := &Client{BaseURL: "http://x", HTTPClient: http.DefaultClient}
	if _, err := c.GetProtocolShare(context.Background(), "", "tank", "d"); err == nil {
		t.Fatal("expected error for empty name")
	}
	if _, err := c.GetProtocolShare(context.Background(), "n", "", "d"); err == nil {
		t.Fatal("expected error for empty pool")
	}
}

func TestDeleteProtocolShare(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method %s", r.Method)
		}
		if r.URL.Path != "/api/v1/protocol-shares/share1" {
			t.Errorf("path %q", r.URL.Path)
		}
		if r.URL.Query().Get("pool") != "tank" || r.URL.Query().Get("dataset") != "csi-nfs/share1" {
			t.Errorf("query %v", r.URL.Query())
		}
		writeJSON(t, w, 202, map[string]string{"jobId": "ps-d"})
	}))
	defer srv.Close()

	job, err := newTestClient(t, srv).DeleteProtocolShare(context.Background(), "share1", "tank", "csi-nfs/share1")
	if err != nil || job.ID != "ps-d" {
		t.Fatalf("err=%v job=%+v", err, job)
	}
}

func TestDeleteProtocolShareSurfacesOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("expected no query, got %q", r.URL.RawQuery)
		}
		writeJSON(t, w, 202, map[string]string{"jobId": "ps-d2"})
	}))
	defer srv.Close()

	job, err := newTestClient(t, srv).DeleteProtocolShare(context.Background(), "share1", "", "")
	if err != nil || job.ID != "ps-d2" {
		t.Fatalf("err=%v job=%+v", err, job)
	}
}

func TestDeleteProtocolShareValidation(t *testing.T) {
	c := &Client{BaseURL: "http://x", HTTPClient: http.DefaultClient}
	if _, err := c.DeleteProtocolShare(context.Background(), "", "", ""); err == nil {
		t.Fatal("expected error for empty name")
	}
	if _, err := c.DeleteProtocolShare(context.Background(), "n", "tank", ""); err == nil {
		t.Fatal("expected error for partial pool/dataset")
	}
}
