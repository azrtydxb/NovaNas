package krb5sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"sort"
	"strings"
	"time"

	novanas "github.com/novanas/nova-nas/clients/go/novanas"
)

// PrincipalAPI is the subset of the novanas SDK used by Reconciler. It
// exists so tests can drop in a fake; production wires up *novanas.Client.
type PrincipalAPI interface {
	ListPrincipals(ctx context.Context) ([]string, error)
	CreatePrincipal(ctx context.Context, spec novanas.CreatePrincipalSpec) (*novanas.Principal, error)
	DeletePrincipal(ctx context.Context, name string) error
}

// ServicePrefixes are the principal-name prefixes we never touch under
// any circumstance. The KDC owns these (krbtgt is intrinsic; nfs/host/
// kadmin are deployed by Agent A's bootstrap; K/M is the master key).
var ServicePrefixes = []string{
	"krbtgt/",
	"kadmin/",
	"nfs/",
	"host/",
	"K/M",
}

// IsServicePrincipal reports whether name is owned by the KDC plumbing
// (not a tenant user principal). Sync must never delete these.
func IsServicePrincipal(name string) bool {
	for _, p := range ServicePrefixes {
		if name == p || strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// ExpectedPrincipals returns the canonical set of principal names a user
// should have given the realm and the user's Keycloak attributes.
//
// Rules:
//   - Disabled users get nothing.
//   - Users with one or more `nova-tenant: <id>` attributes get one
//     `<username>/<tenant>@<realm>` per tenant.
//   - Users with NO tenant attribute and `nova-platform-nfs: true` get
//     a bare-username principal `<username>@<realm>`.
//   - Otherwise: nothing.
func ExpectedPrincipals(u KeycloakUser, realm string) []string {
	if !u.Enabled || strings.TrimSpace(u.Username) == "" {
		return nil
	}
	tenants := u.Tenants()
	if len(tenants) == 0 {
		if u.PlatformNFSEnabled() {
			return []string{u.Username + "@" + realm}
		}
		return nil
	}
	out := make([]string, 0, len(tenants))
	seen := map[string]struct{}{}
	for _, t := range tenants {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		name := u.Username + "/" + t + "@" + realm
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// PrincipalUserPattern returns true if the principal name is shaped like a
// sync-managed user principal (either `user@realm` or `user/tenant@realm`)
// for the configured realm, and is not a service principal. The sync loop
// uses this as the gating predicate when considering a KDC principal for
// deletion.
func PrincipalUserPattern(name, realm string) bool {
	if IsServicePrincipal(name) {
		return false
	}
	suffix := "@" + realm
	if !strings.HasSuffix(name, suffix) {
		return false
	}
	local := strings.TrimSuffix(name, suffix)
	if local == "" {
		return false
	}
	// At most one '/' delimiter (instance).
	if strings.Count(local, "/") > 1 {
		return false
	}
	return true
}

// Config controls Reconciler behaviour.
type Config struct {
	Realm string
	// PollInterval is how often Run performs a full reconcile when no
	// admin events have arrived. Zero disables periodic re-reconcile.
	PollInterval time.Duration
	// EventInterval is how often Run polls Keycloak admin events for
	// incremental updates between full reconciles. Zero disables.
	EventInterval time.Duration
	// Logger receives structured logs. Required.
	Logger *slog.Logger
}

// Reconciler is the core sync engine: takes a KeycloakAPI, a PrincipalAPI,
// and a state file, and reconciles the KDC against Keycloak.
type Reconciler struct {
	KC    KeycloakAPI
	KDC   PrincipalAPI
	State *MemState
	Cfg   Config
}

// NewReconciler builds a reconciler. Callers must populate KC, KDC, State,
// and Cfg; this constructor only fills in defaults.
func NewReconciler(kc KeycloakAPI, kdc PrincipalAPI, st *MemState, cfg Config) *Reconciler {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if strings.TrimSpace(cfg.Realm) == "" {
		cfg.Realm = "NOVANAS.LOCAL"
	}
	return &Reconciler{KC: kc, KDC: kdc, State: st, Cfg: cfg}
}

// ReconcileOnce performs a single full sync pass. Returns the set of
// changes applied (or attempted) and any error that prevented completion.
//
// The reconcile is fail-soft per principal: a single create/delete
// failure logs but does not abort the whole pass.
func (r *Reconciler) ReconcileOnce(ctx context.Context) (Result, error) {
	users, err := r.KC.ListUsers(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("list keycloak users: %w", err)
	}
	kdcList, err := r.KDC.ListPrincipals(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("list kdc principals: %w", err)
	}

	// Build the desired set, indexed by principal -> source user UUID.
	desired := map[string]string{} // principalName -> userUUID
	desiredByUser := map[string][]string{}
	for _, u := range users {
		exp := ExpectedPrincipals(u, r.Cfg.Realm)
		if len(exp) == 0 {
			continue
		}
		desiredByUser[u.ID] = exp
		for _, p := range exp {
			desired[p] = u.ID
		}
	}

	have := map[string]struct{}{}
	for _, n := range kdcList {
		have[n] = struct{}{}
	}

	var res Result

	// Creates: principals in desired but not in have.
	createNames := make([]string, 0, len(desired))
	for p := range desired {
		if _, ok := have[p]; !ok {
			createNames = append(createNames, p)
		}
	}
	sort.Strings(createNames)
	for _, name := range createNames {
		_, err := r.KDC.CreatePrincipal(ctx, novanas.CreatePrincipalSpec{Name: name, Randkey: true})
		if err != nil {
			r.Cfg.Logger.Warn("create principal failed", "principal", name, "err", err)
			res.CreateErrors++
			continue
		}
		res.Created = append(res.Created, name)
		r.Cfg.Logger.Info("created principal", "principal", name)
	}

	// Deletes: principals in the KDC matching the user pattern that have
	// no Keycloak counterpart. Service principals are excluded by
	// PrincipalUserPattern.
	deleteCandidates := make([]string, 0)
	for n := range have {
		if !PrincipalUserPattern(n, r.Cfg.Realm) {
			continue
		}
		if _, wanted := desired[n]; wanted {
			continue
		}
		deleteCandidates = append(deleteCandidates, n)
	}
	sort.Strings(deleteCandidates)
	for _, name := range deleteCandidates {
		if err := r.KDC.DeletePrincipal(ctx, name); err != nil {
			r.Cfg.Logger.Warn("delete principal failed", "principal", name, "err", err)
			res.DeleteErrors++
			continue
		}
		res.Deleted = append(res.Deleted, name)
		r.Cfg.Logger.Info("deleted principal", "principal", name)
	}

	// Update state.
	for uid, ps := range desiredByUser {
		r.State.SetUserPrincipals(uid, ps)
	}
	// Drop UUIDs the user list no longer mentions.
	currentUsers := map[string]struct{}{}
	for _, u := range users {
		currentUsers[u.ID] = struct{}{}
	}
	for uid := range r.State.Snapshot().UserPrincipals {
		if _, ok := currentUsers[uid]; !ok {
			r.State.SetUserPrincipals(uid, nil)
		}
	}
	r.State.MarkSynced(time.Now())
	res.UsersConsidered = len(users)
	res.PrincipalsDesired = len(desired)
	res.PrincipalsInKDC = len(kdcList)
	return res, nil
}

// Result summarises a single reconcile pass.
type Result struct {
	UsersConsidered   int
	PrincipalsDesired int
	PrincipalsInKDC   int
	Created           []string
	Deleted           []string
	CreateErrors      int
	DeleteErrors      int
}

// Run executes the periodic reconcile loop until ctx is cancelled. It
// performs one full reconcile immediately, then alternates between
// periodic full reconciles (Cfg.PollInterval) and admin-event polling
// (Cfg.EventInterval) for incremental updates.
//
// Failures (Keycloak or KDC unreachable) trigger exponential backoff
// capped at 60s; the loop never exits on error. The `firstReady` callback,
// if non-nil, is invoked once after the first successful full reconcile —
// the daemon uses this to send sd_notify READY=1.
func (r *Reconciler) Run(ctx context.Context, firstReady func()) error {
	if r.Cfg.PollInterval == 0 {
		r.Cfg.PollInterval = 5 * time.Minute
	}
	if r.Cfg.EventInterval == 0 {
		r.Cfg.EventInterval = 30 * time.Second
	}
	if firstReady == nil {
		firstReady = func() {}
	}

	// Phase 1: keep retrying the initial reconcile until success or ctx done.
	attempt := 0
	for {
		res, err := r.ReconcileOnce(ctx)
		if err == nil {
			r.Cfg.Logger.Info("initial reconcile complete",
				"users", res.UsersConsidered,
				"desired", res.PrincipalsDesired,
				"created", len(res.Created),
				"deleted", len(res.Deleted))
			firstReady()
			break
		}
		if errors.Is(err, context.Canceled) {
			return err
		}
		attempt++
		backoff := nextBackoff(attempt)
		r.Cfg.Logger.Warn("initial reconcile failed; retrying", "attempt", attempt, "backoff", backoff.String(), "err", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}

	// Phase 2: periodic loop.
	pollT := time.NewTicker(r.Cfg.PollInterval)
	defer pollT.Stop()
	eventT := time.NewTicker(r.Cfg.EventInterval)
	defer eventT.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-pollT.C:
			if _, err := r.ReconcileOnce(ctx); err != nil {
				r.Cfg.Logger.Warn("periodic reconcile failed", "err", err)
			}
		case <-eventT.C:
			if changed, err := r.processAdminEvents(ctx); err != nil {
				r.Cfg.Logger.Warn("admin events poll failed", "err", err)
			} else if changed {
				if _, err := r.ReconcileOnce(ctx); err != nil {
					r.Cfg.Logger.Warn("event-triggered reconcile failed", "err", err)
				}
			}
		}
	}
}

// processAdminEvents polls Keycloak for new admin events. It returns
// `changed=true` if any event indicates a USER mutation we care about,
// signalling the caller to run a full reconcile.
func (r *Reconciler) processAdminEvents(ctx context.Context) (bool, error) {
	since := r.State.LastEvent()
	events, err := r.KC.ListAdminEvents(ctx, since)
	if err != nil {
		return false, err
	}
	if len(events) == 0 {
		return false, nil
	}
	changed := false
	var maxTime int64
	for _, ev := range events {
		if ev.Time > maxTime {
			maxTime = ev.Time
		}
		if ev.ResourceType != "USER" {
			continue
		}
		switch ev.OperationType {
		case "CREATE", "UPDATE", "DELETE":
			changed = true
		}
	}
	if maxTime > 0 {
		// Keycloak admin-event time is in milliseconds since epoch.
		r.State.MarkEvent(time.UnixMilli(maxTime))
	}
	return changed, nil
}

// nextBackoff returns capped exponential backoff with jitter. attempt is
// 1-indexed; values are 2s, 4s, 8s, 16s, 32s, 60s (capped).
func nextBackoff(attempt int) time.Duration {
	base := time.Duration(math.Min(float64(60*time.Second), float64(2*time.Second)*math.Pow(2, float64(attempt-1))))
	// up to 1s jitter
	return base + time.Duration(rand.Int64N(int64(time.Second)))
}
