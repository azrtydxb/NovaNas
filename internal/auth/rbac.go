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
	},
	"nova-operator": {
		PermStorageRead, PermStorageWrite,
		PermNetworkRead,
		PermSystemRead,
		PermSchedulerRead, PermSchedulerWrite,
	},
	"nova-viewer": {
		PermStorageRead,
		PermNetworkRead,
		PermSystemRead,
		PermAuditRead,
		PermSchedulerRead,
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
