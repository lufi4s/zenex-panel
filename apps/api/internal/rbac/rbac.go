// Package rbac defines panel roles and the permissions each role holds.
//
// Design rules:
//   - Customers may only act on resources they own (enforced by handlers via
//     ownership checks; this package answers only "may this role do X at all").
//   - Support may run site-level support actions but never holds node-root,
//     agent-configuration, or secret-read permissions.
//   - Only SecurityAdministrator may release quarantined files or change
//     security rules.
package rbac

type Role string

const (
	RoleCustomer              Role = "customer"
	RoleSupport               Role = "support"
	RoleAdministrator         Role = "administrator"
	RoleSecurityAdministrator Role = "security_administrator"
)

type Permission string

const (
	PermSitesRead          Permission = "sites.read"
	PermSitesCreate        Permission = "sites.create"
	PermSitesDelete        Permission = "sites.delete"
	PermSitesSupportAction Permission = "sites.support_action" // restart PHP-FPM, clear cache
	PermBackupsRun         Permission = "backups.run"
	PermBackupsRestore     Permission = "backups.restore"
	PermDNSManage          Permission = "dns.manage"
	PermSecurityView       Permission = "security.view"
	PermSecurityScan       Permission = "security.scan"
	PermQuarantineRelease  Permission = "quarantine.release"
	PermSecurityRulesEdit  Permission = "security.rules_edit"
	PermNodesRegister      Permission = "nodes.register"
	PermNodesManage        Permission = "nodes.manage"
	PermAgentUpdate        Permission = "agent.update"
	PermUsersManage        Permission = "users.manage"
	PermAuditRead          Permission = "audit.read"
	PermSMTPManage         Permission = "smtp.manage"
)

var matrix = map[Role]map[Permission]bool{
	RoleCustomer: {
		PermSitesRead:          true,
		PermSitesCreate:        true,
		PermSitesDelete:        true,
		PermSitesSupportAction: true,
		PermBackupsRun:         true,
		PermBackupsRestore:     true,
		PermDNSManage:          true,
		PermSecurityView:       true,
		PermSecurityScan:       true,
		PermSMTPManage:         true,
	},
	RoleSupport: {
		PermSitesRead:          true,
		PermSitesSupportAction: true,
		PermSecurityView:       true,
		PermAuditRead:          true,
	},
	RoleAdministrator: {
		PermSitesRead:          true,
		PermSitesCreate:        true,
		PermSitesDelete:        true,
		PermSitesSupportAction: true,
		PermBackupsRun:         true,
		PermBackupsRestore:     true,
		PermDNSManage:          true,
		PermSecurityView:       true,
		PermSecurityScan:       true,
		PermNodesRegister:      true,
		PermNodesManage:        true,
		PermAgentUpdate:        true,
		PermUsersManage:        true,
		PermAuditRead:          true,
		PermSMTPManage:         true,
	},
	RoleSecurityAdministrator: {
		PermSitesRead:          true,
		PermSitesSupportAction: true,
		PermBackupsRun:         true,
		PermSecurityView:       true,
		PermSecurityScan:       true,
		PermQuarantineRelease:  true,
		PermSecurityRulesEdit:  true,
		PermAuditRead:          true,
	},
}

// Can reports whether role holds perm. Unknown roles hold nothing.
func Can(role Role, perm Permission) bool {
	return matrix[role][perm]
}

// Valid reports whether r is a known role.
func Valid(r Role) bool {
	_, ok := matrix[r]
	return ok
}
