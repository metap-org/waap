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
	apexDomain := field("apexDomain", "Apex Domain", metadata.KindString, true, true)
	apexDomain.Unique = boolPtr(true)
	// A domain belongs to one tenant platform-wide, not once per tenant.
	globalScope := metadata.UniqueScopeGlobal
	apexDomain.UniqueScope = &globalScope
	apexDomain.Searchable = boolPtr(true)

	return metadata.EntityDefinition{
		Name:      "waf.domains",
		Label:     "Domain",
		TableName: reconciler.QualifiedTableNameIn("waf.domains", "waf"),
		Fields: []metadata.EntityField{
			apexDomain,
			// Auto-generated server-side (guardedbackend.resolveDomainID) when a zone's hostname
			// resolves to a new apex domain — never entered by hand.
			field("verificationToken", "Verification Token", metadata.KindString, true, false),
			enumField("verificationMethod", "Verification Method", []string{"dnsTxt", "httpFile"}, true, true),
			enumField("verificationStatus", "Verification Status", []string{"unverified", "verified"}, true, true),
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
