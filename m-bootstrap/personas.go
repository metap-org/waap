// Package bootstrap declares the WAAP roles and what each may do, applied to a tenant by
// cmd/waf-bootstrap. It is a service directory of its own (m-bootstrap) because a role's grants
// span entities owned by several services; the one-shot binary blank-imports them all so the
// wildcards expand against the whole product.
//
// Tenant Admin is the built-in "admin" role (bypasses every policy); Platform Admin is not a
// per-tenant role (dev-tools bootstrap-platform-admin). Neither is declared here. Field-level
// permission stays at metap's allow-by-default.
package bootstrap

import (
	"github.com/metap-org/metap/pkg/bootstrap"
	"github.com/metap-org/metap/pkg/metadata"

	"github.com/metap-org/waap/m-management/analytics"
	"github.com/metap-org/waap/model"
)

const (
	RoleSecurityAnalyst = "security_analyst"
	RoleDeveloper       = "developer"
	RoleViewer          = "viewer"
)

func init() {
	bootstrap.Submit(Personas)
	// waf.security_events is a read model served from the Analytics plane, so no service submits it
	// (management-service registers it itself as an external entity). The one-shot bootstrap must
	// still know it, or `waf.*` would expand without it and nobody but an admin could be granted
	// read on it: the read model checks the permission (analytics.Backend).
	metadata.SubmitEntity(analytics.SecurityEventEntity)
}

// Personas is docs/management's persona table as grants.
func Personas() bootstrap.Bootstrap {
	return bootstrap.Bootstrap{
		Roles: []string{RoleSecurityAnalyst, RoleDeveloper, RoleViewer},
		Grants: []bootstrap.Grant{
			// Security Analyst / SOC: sees everything, edits rules and policies, handles
			// incidents and alerts, cannot delete a Zone or touch its domain.
			{Role: RoleSecurityAnalyst, Entities: []string{"waf.*"}, Actions: []string{"read"}},
			{Role: RoleSecurityAnalyst, Entities: []string{model.FirewallRules, model.DdosPolicies, model.IPAccessLists},
				Actions: []string{"create", "update", "delete"}},
			{Role: RoleSecurityAnalyst, Entities: []string{model.Incidents, model.ScanFindings, model.FalsePositives}, Actions: []string{"update", "transition"}},
			{Role: RoleSecurityAnalyst, Entities: []string{model.FalsePositives}, Actions: []string{"create"}},
			{Role: RoleSecurityAnalyst, Entities: []string{model.AlertPolicies}, Actions: []string{"create", "update"}},

			// Developer: owns remediation of scan findings, no DDoS policy or firewall rule access.
			{Role: RoleDeveloper, Entities: []string{model.ScanFindings}, Actions: []string{"read", "update", "transition"}},
			{Role: RoleDeveloper, Entities: []string{model.ScanJobs}, Actions: []string{"read"}},

			// The events of one zone, for a role held only on that zone (the entity grants above cover
			// every zone); read by analytics.ReadTokens through PermissionService.ScopeFor.
			{Role: RoleSecurityAnalyst, Resources: []string{analytics.EventsResource}, Actions: []string{"read"}},
			{Role: RoleViewer, Resources: []string{analytics.EventsResource}, Actions: []string{"read"}},

			// Viewer: read-only incidents, alert history and the security-event analytics.
			{Role: RoleViewer, Entities: []string{model.Incidents, model.AlertNotifications, model.SecurityEvents}, Actions: []string{"read"}},
		},
	}
}
