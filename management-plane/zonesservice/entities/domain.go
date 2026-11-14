// waf.domains — the apex domain a Zone (subdomain) hangs off. Ownership verification
// (verificationToken/verificationMethod/verificationStatus) lives here: proven once per apex
// domain, and every Zone under it inherits that via Zone.verificationStatus, a technical mirror
// field kept in sync by routes.VerifyDomainDNS's cascade.
package entities

import (
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/reconciler"
)

func DomainEntity() metadata.EntityDefinition {
	apexDomain := metadata.NewField("apexDomain", "Apex Domain", metadata.KindString).Require().Index()
	apexDomain.Unique = metadata.Ptr(true)
	// A domain belongs to one tenant platform-wide, not once per tenant.
	globalScope := metadata.UniqueScopeGlobal
	apexDomain.UniqueScope = &globalScope
	apexDomain.Searchable = metadata.Ptr(true)

	return metadata.EntityDefinition{
		Name:      "waf.domains",
		Label:     "Domain",
		TableName: reconciler.QualifiedTableNameIn("waf.domains", "waf"),
		Fields: []metadata.EntityField{
			apexDomain,
			// Auto-generated server-side (guardedbackend.resolveDomainID) when a zone's hostname
			// resolves to a new apex domain — never entered by hand.
			metadata.NewField("verificationToken", "Verification Token", metadata.KindString).Require(),
			metadata.EnumField("verificationMethod", "Verification Method", "dnsTxt", "httpFile").Require().Index(),
			metadata.EnumField("verificationStatus", "Verification Status", "unverified", "verified").Require().Index(),
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields: []string{"apexDomain", "verificationStatus"}, Filters: []string{"apexDomain", "verificationStatus"},
			MaxLimit: 100,
		}},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
	}
}

func init() { metadata.SubmitEntity(DomainEntity) }
