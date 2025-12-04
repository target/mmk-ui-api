package authroles

import (
	domainauth "github.com/target/mmk-ui-api/internal/domain/auth"
)

// StaticRoleMapper maps groups by simple string membership rules.
// Move of logic from internal/mocks/auth to a concrete adapter for production wiring.
type StaticRoleMapper struct {
	AdminGroup string
	UserGroup  string
}

func (m StaticRoleMapper) Map(groups []string) domainauth.Role {
	for _, g := range groups {
		if m.AdminGroup != "" && g == m.AdminGroup {
			return domainauth.RoleAdmin
		}
	}
	for _, g := range groups {
		if m.UserGroup != "" && g == m.UserGroup {
			return domainauth.RoleUser
		}
	}
	return domainauth.RoleGuest
}
