package auth

// Permission is a coarse capability the API checks. Multiple Permissions
// can be required for an operation (treated as OR — any matching role
// grants access).
type Permission string

const (
	PermStorageRead    Permission = "nova:storage:read"
	PermStorageWrite   Permission = "nova:storage:write"
	PermNetworkRead    Permission = "nova:network:read"
	PermNetworkWrite   Permission = "nova:network:write"
	PermSystemRead     Permission = "nova:system:read"
	PermSystemWrite    Permission = "nova:system:write"
	PermSystemAdmin    Permission = "nova:system:admin" // reboot/shutdown
	PermAuditRead      Permission = "nova:audit:read"
	PermSchedulerRead  Permission = "nova:scheduler:read"
	PermSchedulerWrite Permission = "nova:scheduler:write"
	// Krb5 KDC management. PermKrb5Write is sensitive — it implicitly
	// grants the holder the ability to mint service keytabs.
	PermKrb5Read  Permission = "nova:krb5:read"
	PermKrb5Write Permission = "nova:krb5:write"
	// Notifications: SMTP relay configuration and test sends. Write is
	// admin-only because it can both leak credentials (read-back is
	// redacted, but the manager still holds the cleartext) and be used
	// to phish (the operator chooses the From address).
	PermNotificationsRead  Permission = "nova:notifications:read"
	PermNotificationsWrite Permission = "nova:notifications:write"
	// Notification Center event stream (the bell). Read covers list /
	// unread-count / SSE subscribe; Write covers per-user state mutations
	// (mark-read, dismiss, snooze) on the caller's OWN events. The
	// underlying notifications themselves are never user-mutable; only
	// per-user state is — granted broadly (viewer+).
	PermNotificationsEventsRead  Permission = "nova:notifications.events:read"
	PermNotificationsEventsWrite Permission = "nova:notifications.events:write"
	// Pool/dataset native-encryption management. Read covers status
	// (is encrypted? algorithm?). Write covers initialize / load-key /
	// unload-key. Recover is the break-glass capability that exposes
	// the raw 32-byte ZFS key — it is admin-only and every call is
	// audit-logged by the API handler.
	PermPoolEncryptionRead    Permission = "nova:encryption:read"
	PermPoolEncryptionWrite   Permission = "nova:encryption:write"
	PermPoolEncryptionRecover Permission = "nova:encryption:recover"
	// Replication subsystem (general — covers ZFS native, S3, and
	// rsync-over-SSH backends). Read covers job listing/detail and run
	// history. Write covers CRUD plus the ad-hoc /run trigger.
	PermReplicationRead  Permission = "nova:replication:read"
	PermReplicationWrite Permission = "nova:replication:write"
	// Scrub policy management. Read covers listing policies and the
	// last-fired-at metadata. Write covers policy CRUD AND the ad-hoc
	// per-pool scrub trigger (operator+ for the trigger so on-call
	// engineers can kick a scrub without admin rights).
	PermScrubRead  Permission = "nova:scrub:read"
	PermScrubWrite Permission = "nova:scrub:write"

	// Alerts: pass-through to Alertmanager. Read covers alert + silence
	// listing; Write covers silence create/expire.
	PermAlertsRead  Permission = "nova:alerts:read"
	PermAlertsWrite Permission = "nova:alerts:write"

	// Logs: pass-through to Loki. LogQL queries can disclose sensitive
	// payloads, so this is read-only and operator+ by default (viewer
	// is intentionally excluded).
	PermLogsRead Permission = "nova:logs:read"

	// Sessions / login history. Reading the caller's OWN sessions and
	// login history is granted to viewer+; reading or revoking ANOTHER
	// user's sessions requires SessionsAdmin (admin-only).
	PermSessionsRead  Permission = "nova:sessions:read"
	PermSessionsAdmin Permission = "nova:sessions:admin"

	// KubeVirt VM management. Read covers list/get plus console-session
	// minting (the WebSocket URL is a credential — admin+operator+viewer
	// all need a way to view a VM's screen). Write covers CRUD plus
	// lifecycle (start/stop/restart/pause/migrate) and snapshot/restore.
	PermVMRead  Permission = "nova:vm:read"
	PermVMWrite Permission = "nova:vm:write"

	// Workloads (Apps) subsystem. Read covers the curated chart catalog
	// and listing installed apps (Helm releases on the embedded k3s).
	// Write covers install/upgrade/uninstall/rollback. Granted to
	// viewer+ for read, operator+ for write — consistent with other
	// domains.
	PermWorkloadsRead  Permission = "nova:workloads:read"
	PermWorkloadsWrite Permission = "nova:workloads:write"
)

// RoleMap maps Keycloak role names to a set of Permissions. Operators
// configure their realm to assign roles; this map says what each role
// can do.
type RoleMap map[string][]Permission

// DefaultRoleMap is what NovaNAS ships with. Operators can override.
var DefaultRoleMap = RoleMap{
	"nova-admin": {
		PermStorageRead, PermStorageWrite,
		PermNetworkRead, PermNetworkWrite,
		PermSystemRead, PermSystemWrite, PermSystemAdmin,
		PermAuditRead,
		PermSchedulerRead, PermSchedulerWrite,
		PermKrb5Read, PermKrb5Write,
		PermNotificationsRead, PermNotificationsWrite,
		PermNotificationsEventsRead, PermNotificationsEventsWrite,
		PermPoolEncryptionRead, PermPoolEncryptionWrite, PermPoolEncryptionRecover,
		PermReplicationRead, PermReplicationWrite,
		PermScrubRead, PermScrubWrite,
		PermAlertsRead, PermAlertsWrite,
		PermLogsRead,
		PermSessionsRead, PermSessionsAdmin,
		PermVMRead, PermVMWrite,
		PermWorkloadsRead, PermWorkloadsWrite,
	},
	"nova-operator": {
		PermStorageRead, PermStorageWrite,
		PermNetworkRead,
		PermSystemRead,
		PermAuditRead,
		PermSchedulerRead, PermSchedulerWrite,
		PermNotificationsRead,
		PermNotificationsEventsRead, PermNotificationsEventsWrite,
		PermPoolEncryptionRead, PermPoolEncryptionWrite,
		PermReplicationRead, PermReplicationWrite,
		PermScrubRead, PermScrubWrite,
		PermAlertsRead, PermAlertsWrite,
		PermLogsRead,
		PermSessionsRead,
		PermVMRead, PermVMWrite,
		PermWorkloadsRead, PermWorkloadsWrite,
	},
	// Viewers intentionally do NOT receive PermAuditRead. The audit log
	// reveals who has accessed which resources; granting it to read-only
	// viewers leaks reconnaissance signal. Operator and admin only.
	"nova-viewer": {
		PermStorageRead,
		PermNetworkRead,
		PermSystemRead,
		PermSchedulerRead,
		PermReplicationRead,
		PermScrubRead,
		PermAlertsRead,
		PermSessionsRead,
		PermVMRead,
		PermWorkloadsRead,
		PermNotificationsEventsRead, PermNotificationsEventsWrite,
	},
	"nova-system-admin": {
		PermStorageRead,
		PermNetworkRead, PermNetworkWrite,
		PermSystemRead, PermSystemWrite, PermSystemAdmin,
	},
}

// IdentityHasPermission returns true if any of identity's roles grants p.
func IdentityHasPermission(roleMap RoleMap, identity *Identity, p Permission) bool {
	if identity == nil || roleMap == nil {
		return false
	}
	for _, role := range identity.Roles {
		perms, ok := roleMap[role]
		if !ok {
			continue
		}
		for _, granted := range perms {
			if granted == p {
				return true
			}
		}
	}
	return false
}
