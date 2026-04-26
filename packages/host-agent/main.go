// Generic NovaNas config-poller sidecar (#55).
//
// Designed to run alongside a daemon (nfs-ganesha, samba, keepalived,
// iscsi-target, nvmeof-target, …) inside the same Pod or on the host.
// On a fixed interval it:
//
//  1. GETs an authoritative resource list from the NovaNas API.
//  2. Renders a Go text/template against that list, into an output
//     file the daemon reads.
//  3. If the rendered output changed, runs a reload command (default:
//     SIGHUP to a pid file) so the daemon picks up the new config.
//
// Identity: presents the projected ServiceAccount JWT as a bearer
// token. Api-side TokenReview maps it onto an internal: role (see
// packages/api/src/auth/tokenreview.ts and #50/#55 SA mappings).
//
// The poller is intentionally generic — the same binary works for
// every host-side daemon. The template is the only thing that varies
// per consumer. Templates live next to each daemon's chart wiring as
// a ConfigMap mounted into the Pod; the daemon's own config files
// stay on an emptyDir/hostPath volume the poller writes into.

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"text/template"
	"time"

	novanas "github.com/azrtydxb/novanas/packages/sdk/go-client"
)

func main() {
	var (
		apiURL     = flag.String("api-url", os.Getenv("NOVANAS_API_URL"), "NovaNas API base URL")
		tokenFile  = flag.String("token-file", os.Getenv("NOVANAS_API_TOKEN_FILE"), "path to projected SA token (defaults to in-pod path)")
		resource   = flag.String("resource", "", "resource path under /api/v1, e.g. 'nfs-servers'")
		tmplPath   = flag.String("template", "", "path to a Go text/template")
		outputPath = flag.String("output", "", "path to write rendered config")
		reloadCmd  = flag.String("reload-cmd", "", "shell command run after a config change; defaults to SIGHUP via pidfile")
		pidFile    = flag.String("pidfile", "", "path to a daemon pid file (used by the default reload)")
		interval   = flag.Duration("interval", 30*time.Second, "poll interval")
	)
	flag.Parse()

	if *resource == "" || *tmplPath == "" || *outputPath == "" {
		log.Fatal("flags -resource, -template and -output are all required")
	}
	if *apiURL == "" {
		log.Fatal("NOVANAS_API_URL is unset (and -api-url is empty)")
	}

	tmpl, err := template.New(filepath.Base(*tmplPath)).
		Funcs(template.FuncMap{"trim": func(s string) string { return s }}).
		ParseFiles(*tmplPath)
	if err != nil {
		log.Fatalf("parse template %s: %v", *tmplPath, err)
	}

	tokenPath := *tokenFile
	if tokenPath == "" {
		tokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	}

	p := &poller{
		apiURL:     *apiURL,
		tokenPath:  tokenPath,
		resource:   *resource,
		template:   tmpl,
		outputPath: *outputPath,
		reloadCmd:  *reloadCmd,
		pidFile:    *pidFile,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	t := time.NewTicker(*interval)
	defer t.Stop()
	log.Printf("started: poll %s/api/v1/%s every %s", *apiURL, *resource, *interval)
	p.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			log.Printf("shutdown")
			return
		case <-t.C:
			p.tick(ctx)
		}
	}
}

// decodeJSON reads and decodes the response body. Body is fully read
// before the connection is reused so http.Client can pool it.
func decodeJSON(resp *http.Response, out any) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

type poller struct {
	apiURL     string
	tokenPath  string
	resource   string
	template   *template.Template
	outputPath string
	reloadCmd  string
	pidFile    string

	lastHash [32]byte
}

func (p *poller) tick(ctx context.Context) {
	tok, err := os.ReadFile(p.tokenPath)
	if err != nil {
		log.Printf("read token: %v", err)
		return
	}
	items, err := p.fetch(ctx, string(bytes.TrimSpace(tok)))
	if err != nil {
		log.Printf("fetch: %v", err)
		return
	}

	var rendered bytes.Buffer
	if err := p.template.Execute(&rendered, map[string]any{
		"Items": items,
	}); err != nil {
		log.Printf("render: %v", err)
		return
	}
	hash := sha256.Sum256(rendered.Bytes())
	if hash == p.lastHash {
		return // unchanged
	}

	tmpFile := p.outputPath + ".tmp"
	if err := os.WriteFile(tmpFile, rendered.Bytes(), 0o600); err != nil {
		log.Printf("write tmp: %v", err)
		return
	}
	if err := os.Rename(tmpFile, p.outputPath); err != nil {
		log.Printf("rename to %s: %v", p.outputPath, err)
		return
	}
	p.lastHash = hash
	log.Printf("config updated: %s (%d bytes, %d items)", p.outputPath, rendered.Len(), len(items))

	if err := p.reload(ctx); err != nil {
		log.Printf("reload: %v", err)
	}
}

// fetch issues GET /api/v1/<resource> via the SDK's underlying do()
// path. Items are returned as opaque map[string]any so the template
// can reach into spec/status without per-resource Go bindings.
func (p *poller) fetch(ctx context.Context, token string) ([]map[string]any, error) {
	client := novanas.New(p.apiURL, token)
	// Use the raw http transport since each daemon has its own
	// resource shape. The SDK's typed helpers (ListPools etc.) are
	// for the storage controllers; the sidecar is intentionally
	// untyped here so adding new daemons doesn't require SDK PRs.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		p.apiURL+"/api/v1/"+p.resource, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("api %s -> %d", p.resource, resp.StatusCode)
	}
	var envelope struct {
		Items []map[string]any `json:"items"`
	}
	if err := decodeJSON(resp, &envelope); err != nil {
		return nil, err
	}
	_ = client // reserved for future typed listers
	return envelope.Items, nil
}

func (p *poller) reload(ctx context.Context) error {
	if p.reloadCmd != "" {
		cmd := exec.CommandContext(ctx, "sh", "-c", p.reloadCmd)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	if p.pidFile == "" {
		// Nothing to do: the daemon is configured to re-read on its
		// own (e.g. Samba inotify on smb.conf in some distros).
		return nil
	}
	raw, err := os.ReadFile(p.pidFile)
	if err != nil {
		return fmt.Errorf("read pidfile %s: %w", p.pidFile, err)
	}
	pid, err := strconv.Atoi(string(bytes.TrimSpace(raw)))
	if err != nil {
		return fmt.Errorf("parse pid %q: %w", string(raw), err)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGHUP)
}
