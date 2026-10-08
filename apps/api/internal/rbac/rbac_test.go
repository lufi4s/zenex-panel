package rbac

import "testing"

func TestCustomerCannotTouchNodesOrSecretsPermissions(t *testing.T) {
	for _, p := range []Permission{PermNodesRegister, PermNodesManage, PermAgentUpdate, PermUsersManage, PermQuarantineRelease, PermSecurityRulesEdit} {
		if Can(RoleCustomer, p) {
			t.Errorf("customer must not hold %s", p)
		}
	}
}

func TestSupportHasNoRootOrAgentCapability(t *testing.T) {
	for _, p := range []Permission{PermNodesRegister, PermNodesManage, PermAgentUpdate, PermQuarantineRelease, PermSecurityRulesEdit, PermSitesDelete} {
		if Can(RoleSupport, p) {
			t.Errorf("support must not hold %s", p)
		}
	}
	if !Can(RoleSupport, PermSitesSupportAction) {
		t.Error("support should be able to run site support actions")
	}
}

func TestOnlySecurityAdminReleasesQuarantine(t *testing.T) {
	for _, r := range []Role{RoleCustomer, RoleSupport, RoleAdministrator} {
		if Can(r, PermQuarantineRelease) {
			t.Errorf("%s must not release quarantine", r)
		}
	}
	if !Can(RoleSecurityAdministrator, PermQuarantineRelease) {
		t.Error("security administrator must release quarantine")
	}
}

func TestUnknownRole(t *testing.T) {
	if Can(Role("root"), PermSitesRead) || Valid(Role("root")) {
		t.Error("unknown role granted access")
	}
}
