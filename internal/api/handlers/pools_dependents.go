// Package handlers — pool dependents endpoint.
//
// Dependents reports every entity that references a pool, so the GUI
// can prevent destructive deletion until the user has cleaned up.
// Sources surveyed: child datasets, protocol shares, iSCSI targets,
// replication jobs/schedules, snapshot schedules, scrub policies, and
// installed plugins with provisioned datasets in the pool.
//
// Each source is queried independently and a failure to read one does
// NOT fail the whole endpoint — partial dependent lists are more
// useful to the operator than a generic 500.
package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/iscsi"
	"github.com/novanas/nova-nas/internal/host/protocolshare"
	"github.com/novanas/nova-nas/internal/plugins"
	"github.com/novanas/nova-nas/internal/store"
)

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}

// PoolDependent describes a single thing referencing a pool. The UI
// uses Kind to group by category and Blocking to decide whether the
// pool delete confirmation can be enabled.
type PoolDependent struct {
	Kind     string `json:"kind"`     // dataset|share|iscsi-target|replication-job|replication-schedule|snapshot-schedule|scrub-policy|plugin
	ID       string `json:"id"`       // primary key the deletion endpoint expects
	Name     string `json:"name"`     // display name
	Detail   string `json:"detail,omitempty"`
	Enabled  *bool  `json:"enabled,omitempty"`
	Blocking bool   `json:"blocking"` // true → must be removed before destroying the pool
}

// PoolDependentsResponse is the body of GET /pools/{name}/dependents.
type PoolDependentsResponse struct {
	Pool       string          `json:"pool"`
	Dependents []PoolDependent `json:"dependents"`
}

// PoolDependentsHandler aggregates dependent surveys across the
// subsystems wired into the API. All fields except Logger and Pool
// are optional — when nil the corresponding source is skipped, which
// is what tests and minimally-wired dev servers want.
type PoolDependentsHandler struct {
	Logger        *slog.Logger
	Datasets      DatasetManager
	ProtocolShare *protocolshare.Manager
	Iscsi         *iscsi.Manager
	Plugins       *plugins.Manager
	Store         *store.Store
}

// Handle GET /pools/{name}/dependents.
func (h *PoolDependentsHandler) Handle(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "pool name is required")
		return
	}
	out := []PoolDependent{}
	out = append(out, h.collectDatasets(r.Context(), name)...)
	out = append(out, h.collectShares(r.Context(), name)...)
	out = append(out, h.collectIscsi(r.Context(), name)...)
	out = append(out, h.collectReplicationJobs(r.Context(), name)...)
	out = append(out, h.collectReplicationSchedules(r.Context(), name)...)
	out = append(out, h.collectSnapshotSchedules(r.Context(), name)...)
	out = append(out, h.collectScrubPolicies(r.Context(), name)...)
	out = append(out, h.collectPlugins(r.Context(), name)...)
	middleware.WriteJSON(w, h.Logger, http.StatusOK, PoolDependentsResponse{
		Pool:       name,
		Dependents: out,
	})
}

// inPool reports whether a dataset path belongs to the named pool.
// Matches both the pool root ("tank") and any descendant ("tank/...").
func inPool(dataset, pool string) bool {
	if dataset == pool {
		return true
	}
	return strings.HasPrefix(dataset, pool+"/")
}

func (h *PoolDependentsHandler) collectDatasets(ctx context.Context, pool string) []PoolDependent {
	if h.Datasets == nil {
		return nil
	}
	ds, err := h.Datasets.List(ctx, pool)
	if err != nil {
		h.Logger.Warn("dependents: datasets list failed", "pool", pool, "err", err)
		return nil
	}
	out := make([]PoolDependent, 0, len(ds))
	for _, d := range ds {
		// The pool root itself is the pool — skip; only count children.
		if d.Name == pool {
			continue
		}
		out = append(out, PoolDependent{
			Kind:     "dataset",
			ID:       d.Name,
			Name:     d.Name,
			Detail:   d.Mountpoint,
			Blocking: false, // zpool destroy cascades
		})
	}
	return out
}

func (h *PoolDependentsHandler) collectShares(ctx context.Context, pool string) []PoolDependent {
	if h.ProtocolShare == nil {
		return nil
	}
	shares, err := h.ProtocolShare.List(ctx)
	if err != nil {
		h.Logger.Warn("dependents: shares list failed", "pool", pool, "err", err)
		return nil
	}
	out := []PoolDependent{}
	for _, s := range shares {
		if s.Pool != pool {
			continue
		}
		protos := make([]string, 0, len(s.Protocols))
		for _, p := range s.Protocols {
			protos = append(protos, string(p))
		}
		out = append(out, PoolDependent{
			Kind:     "share",
			ID:       s.Name,
			Name:     s.Name,
			Detail:   strings.ToUpper(strings.Join(protos, ", ")) + " · " + s.DatasetName,
			Blocking: true,
		})
	}
	return out
}

func (h *PoolDependentsHandler) collectIscsi(ctx context.Context, pool string) []PoolDependent {
	if h.Iscsi == nil {
		return nil
	}
	targets, err := h.Iscsi.ListTargets(ctx)
	if err != nil {
		h.Logger.Warn("dependents: iscsi targets list failed", "pool", pool, "err", err)
		return nil
	}
	zvolPrefix := "/dev/zvol/" + pool + "/"
	zvolRoot := "/dev/zvol/" + pool
	out := []PoolDependent{}
	for _, t := range targets {
		detail, err := h.Iscsi.GetTarget(ctx, t.IQN)
		if err != nil || detail == nil {
			continue
		}
		var matched []string
		for _, lun := range detail.LUNs {
			if strings.HasPrefix(lun.Zvol, zvolPrefix) || lun.Zvol == zvolRoot {
				matched = append(matched, lun.Zvol)
			}
		}
		if len(matched) == 0 {
			continue
		}
		out = append(out, PoolDependent{
			Kind:     "iscsi-target",
			ID:       t.IQN,
			Name:     t.IQN,
			Detail:   strings.Join(matched, ", "),
			Blocking: true,
		})
	}
	return out
}

func (h *PoolDependentsHandler) collectReplicationJobs(ctx context.Context, pool string) []PoolDependent {
	if h.Store == nil || h.Store.Queries == nil {
		return nil
	}
	jobs, err := h.Store.Queries.ListReplicationJobs(ctx)
	if err != nil {
		h.Logger.Warn("dependents: replication jobs list failed", "pool", pool, "err", err)
		return nil
	}
	out := []PoolDependent{}
	for _, j := range jobs {
		// Source/destination shapes vary by backend; we look for the
		// pool name as a JSON value substring. False positives on
		// stray strings are unlikely because the JSON values are
		// either dataset paths or hostnames.
		hit := jsonReferencesPool(j.SourceJson, pool) || jsonReferencesPool(j.DestinationJson, pool)
		if !hit {
			continue
		}
		enabled := j.Enabled
		out = append(out, PoolDependent{
			Kind:     "replication-job",
			ID:       uuidString(j.ID),
			Name:     j.Name,
			Detail:   j.Backend + " · " + j.Direction,
			Enabled:  &enabled,
			Blocking: true,
		})
	}
	return out
}

func (h *PoolDependentsHandler) collectReplicationSchedules(ctx context.Context, pool string) []PoolDependent {
	if h.Store == nil || h.Store.Queries == nil {
		return nil
	}
	scheds, err := h.Store.Queries.ListReplicationSchedules(ctx)
	if err != nil {
		h.Logger.Warn("dependents: replication schedules list failed", "pool", pool, "err", err)
		return nil
	}
	out := []PoolDependent{}
	for _, s := range scheds {
		if !inPool(s.SrcDataset, pool) {
			continue
		}
		enabled := s.Enabled
		out = append(out, PoolDependent{
			Kind:     "replication-schedule",
			ID:       uuidString(s.ID),
			Name:     s.SrcDataset,
			Detail:   s.Cron,
			Enabled:  &enabled,
			Blocking: true,
		})
	}
	return out
}

func (h *PoolDependentsHandler) collectSnapshotSchedules(ctx context.Context, pool string) []PoolDependent {
	if h.Store == nil || h.Store.Queries == nil {
		return nil
	}
	scheds, err := h.Store.Queries.ListSnapshotSchedules(ctx)
	if err != nil {
		h.Logger.Warn("dependents: snapshot schedules list failed", "pool", pool, "err", err)
		return nil
	}
	out := []PoolDependent{}
	for _, s := range scheds {
		if !inPool(s.Dataset, pool) {
			continue
		}
		enabled := s.Enabled
		out = append(out, PoolDependent{
			Kind:     "snapshot-schedule",
			ID:       uuidString(s.ID),
			Name:     s.Name,
			Detail:   s.Dataset + " · " + s.Cron,
			Enabled:  &enabled,
			Blocking: true,
		})
	}
	return out
}

func (h *PoolDependentsHandler) collectScrubPolicies(ctx context.Context, pool string) []PoolDependent {
	if h.Store == nil || h.Store.Queries == nil {
		return nil
	}
	policies, err := h.Store.Queries.ListScrubPolicies(ctx)
	if err != nil {
		h.Logger.Warn("dependents: scrub policies list failed", "pool", pool, "err", err)
		return nil
	}
	out := []PoolDependent{}
	for _, p := range policies {
		if !poolsListContains(p.Pools, pool) {
			continue
		}
		enabled := p.Enabled
		out = append(out, PoolDependent{
			Kind:     "scrub-policy",
			ID:       uuidString(p.ID),
			Name:     p.Name,
			Detail:   p.Cron,
			Enabled:  &enabled,
			Blocking: true,
		})
	}
	return out
}

func (h *PoolDependentsHandler) collectPlugins(ctx context.Context, pool string) []PoolDependent {
	if h.Plugins == nil {
		return nil
	}
	installed, err := h.Plugins.List(ctx)
	if err != nil {
		h.Logger.Warn("dependents: plugins list failed", "pool", pool, "err", err)
		return nil
	}
	out := []PoolDependent{}
	for _, inst := range installed {
		var matched []string
		for _, r := range inst.Resources {
			if r.Type != plugins.NeedDataset {
				continue
			}
			if inPool(r.ID, pool) {
				matched = append(matched, r.ID)
			}
		}
		if len(matched) == 0 {
			continue
		}
		out = append(out, PoolDependent{
			Kind:     "plugin",
			ID:       inst.Name,
			Name:     inst.Name,
			Detail:   strings.Join(matched, ", "),
			Blocking: true,
		})
	}
	return out
}

// poolsListContains reports whether a comma- or whitespace-separated
// pools field references the named pool. ScrubPolicy.Pools is stored
// as a freeform string for legacy reasons.
func poolsListContains(pools, name string) bool {
	if pools == "" {
		return false
	}
	for _, p := range strings.FieldsFunc(pools, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		if p == name {
			return true
		}
	}
	return false
}

// jsonReferencesPool checks whether a JSON blob mentions the pool as
// a value. We look for both the bare pool name and "<pool>/" because
// the backends store dataset paths as plain strings.
func jsonReferencesPool(b []byte, pool string) bool {
	if len(b) == 0 || pool == "" {
		return false
	}
	s := string(b)
	// Quoted exact match.
	if strings.Contains(s, `"`+pool+`"`) {
		return true
	}
	// Quoted prefix (e.g. "tank/photos").
	return strings.Contains(s, `"`+pool+`/`)
}
