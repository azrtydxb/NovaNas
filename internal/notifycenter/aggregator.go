package notifycenter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// AggregatorConfig holds the polling configuration. Zero values are
// replaced with sensible defaults in NewAggregator.
type AggregatorConfig struct {
	Interval        time.Duration // default 30s
	AlertmanagerURL string        // empty = skip alertmanager polling
	HTTPClient      *http.Client  // optional; defaults to net/http default
}

// SourceQueries is the subset of storedb.Queries used for source
// polling. The aggregator only reads here; writes go through
// Manager.RecordEvent.
type SourceQueries interface {
	ListJobs(ctx context.Context, arg storedb.ListJobsParams) ([]storedb.Job, error)
	SearchAudit(ctx context.Context, arg storedb.SearchAuditParams) ([]storedb.AuditLog, error)
}

// Aggregator periodically polls the three known event sources and
// forwards new entries to a Manager. It tracks per-source cursors in
// memory; on restart the cursor resets to "now", which is acceptable
// because RecordEvent is idempotent on (source, source_id) — a race
// where the aggregator re-observes a startup-window alert produces
// the same DB row, no duplicate.
type Aggregator struct {
	cfg     AggregatorConfig
	mgr     *Manager
	src     SourceQueries
	logger  *slog.Logger
	cursors map[Source]time.Time
}

// NewAggregator wires an aggregator. mgr and src are required; cfg
// may be the zero value (defaults applied).
func NewAggregator(mgr *Manager, src SourceQueries, cfg AggregatorConfig, logger *slog.Logger) *Aggregator {
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	if logger == nil {
		logger = slog.Default()
	}
	now := time.Now().UTC()
	return &Aggregator{
		cfg:    cfg,
		mgr:    mgr,
		src:    src,
		logger: logger,
		cursors: map[Source]time.Time{
			SourceAlertmanager: now,
			SourceJobs:         now,
			SourceAudit:        now,
		},
	}
}

// Run blocks until ctx is cancelled, polling at AggregatorConfig.Interval.
// Errors from a single source poll are logged but do not abort the loop —
// a flaky Alertmanager must not silence job/audit notifications.
func (a *Aggregator) Run(ctx context.Context) error {
	tick := time.NewTicker(a.cfg.Interval)
	defer tick.Stop()
	a.tick(ctx) // run once immediately so the bell isn't empty for 30s on boot
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			a.tick(ctx)
		}
	}
}

func (a *Aggregator) tick(ctx context.Context) {
	if a.cfg.AlertmanagerURL != "" {
		if err := a.pollAlertmanager(ctx); err != nil {
			a.logger.Warn("notifycenter: alertmanager poll", "err", err)
		}
	}
	if a.src != nil {
		if err := a.pollJobs(ctx); err != nil {
			a.logger.Warn("notifycenter: jobs poll", "err", err)
		}
		if err := a.pollAudit(ctx); err != nil {
			a.logger.Warn("notifycenter: audit poll", "err", err)
		}
	}
}

// alertmanagerAlert mirrors the relevant subset of /api/v2/alerts.
// Only the fields we project are decoded.
type alertmanagerAlert struct {
	Fingerprint string            `json:"fingerprint"`
	StartsAt    time.Time         `json:"startsAt"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	GeneratorURL string           `json:"generatorURL"`
}

func (a *Aggregator) pollAlertmanager(ctx context.Context) error {
	cursor := a.cursors[SourceAlertmanager]
	url := strings.TrimRight(a.cfg.AlertmanagerURL, "/") + "/api/v2/alerts"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := a.cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("alertmanager returned %d", resp.StatusCode)
	}
	var alerts []alertmanagerAlert
	if err := json.NewDecoder(resp.Body).Decode(&alerts); err != nil {
		return err
	}
	newest := cursor
	for _, al := range alerts {
		if !al.StartsAt.After(cursor) {
			continue
		}
		if al.StartsAt.After(newest) {
			newest = al.StartsAt
		}
		sev := mapAlertSeverity(al.Labels["severity"])
		title := al.Labels["alertname"]
		if title == "" {
			title = "Alert"
		}
		body := al.Annotations["description"]
		if body == "" {
			body = al.Annotations["summary"]
		}
		if _, err := a.mgr.RecordEvent(ctx, RecordInput{
			Source:   SourceAlertmanager,
			SourceID: al.Fingerprint,
			Severity: sev,
			Title:    title,
			Body:     body,
			Link:     al.GeneratorURL,
		}); err != nil {
			a.logger.Warn("notifycenter: record alert", "err", err, "fingerprint", al.Fingerprint)
		}
	}
	a.cursors[SourceAlertmanager] = newest
	return nil
}

func (a *Aggregator) pollJobs(ctx context.Context) error {
	cursor := a.cursors[SourceJobs]
	failed := "failed"
	rows, err := a.src.ListJobs(ctx, storedb.ListJobsParams{
		Limit: 200, Offset: 0, State: &failed,
	})
	if err != nil {
		return err
	}
	newest := cursor
	for _, j := range rows {
		if !j.FinishedAt.Valid {
			continue
		}
		if !j.FinishedAt.Time.After(cursor) {
			continue
		}
		if j.FinishedAt.Time.After(newest) {
			newest = j.FinishedAt.Time
		}
		title := fmt.Sprintf("Job failed: %s", j.Kind)
		body := j.Target
		if j.Error != nil && *j.Error != "" {
			body = strings.TrimSpace(*j.Error)
		}
		idStr := uuidToString(j.ID)
		if _, err := a.mgr.RecordEvent(ctx, RecordInput{
			Source:   SourceJobs,
			SourceID: idStr,
			Severity: SeverityWarning,
			Title:    title,
			Body:     body,
			Link:     "/jobs/" + idStr,
		}); err != nil {
			a.logger.Warn("notifycenter: record job", "err", err, "job_id", idStr)
		}
	}
	a.cursors[SourceJobs] = newest
	return nil
}

func (a *Aggregator) pollAudit(ctx context.Context) error {
	cursor := a.cursors[SourceAudit]
	rejected := "rejected"
	rows, err := a.src.SearchAudit(ctx, storedb.SearchAuditParams{
		Limit:  200,
		Result: &rejected,
		Since:  pgtype.Timestamptz{Time: cursor, Valid: true},
	})
	if err != nil {
		return err
	}
	newest := cursor
	for _, r := range rows {
		if !r.Ts.Valid {
			continue
		}
		if !r.Ts.Time.After(cursor) {
			continue
		}
		if r.Ts.Time.After(newest) {
			newest = r.Ts.Time
		}
		sev := SeverityInfo
		if isAuthFailure(r.Action) {
			sev = SeverityWarning
		}
		actor := ""
		if r.Actor != nil {
			actor = *r.Actor
		}
		title := fmt.Sprintf("Rejected: %s", r.Action)
		body := strings.TrimSpace(actor + " " + r.Target)
		if _, err := a.mgr.RecordEvent(ctx, RecordInput{
			Source:   SourceAudit,
			SourceID: fmt.Sprintf("%d", r.ID),
			Severity: sev,
			Title:    title,
			Body:     body,
			Link:     "/audit?id=" + fmt.Sprintf("%d", r.ID),
		}); err != nil {
			a.logger.Warn("notifycenter: record audit", "err", err, "audit_id", r.ID)
		}
	}
	a.cursors[SourceAudit] = newest
	return nil
}

// mapAlertSeverity normalizes Alertmanager's severity label.
// Anything we don't recognize is downgraded to info — better to
// quietly surface than to incorrectly page someone.
func mapAlertSeverity(label string) Severity {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "critical", "page":
		return SeverityCritical
	case "warning":
		return SeverityWarning
	}
	return SeverityInfo
}

// isAuthFailure heuristic — actions that touch auth surfaces (login,
// token refresh, role check failure) are escalated to warning so they
// stand out in the bell.
func isAuthFailure(action string) bool {
	a := strings.ToLower(action)
	return strings.Contains(a, "auth") || strings.Contains(a, "login") || strings.Contains(a, "token")
}

func uuidToString(p pgtype.UUID) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		p.Bytes[0:4], p.Bytes[4:6], p.Bytes[6:8], p.Bytes[8:10], p.Bytes[10:16])
}
