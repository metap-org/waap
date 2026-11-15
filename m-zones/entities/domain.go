// waf.domains — the apex domain a Zone (subdomain) hangs off. Ownership verification
// (verificationToken/verificationMethod/verificationStatus) lives here: proven once per apex
// domain, and every Zone under it inherits that via Zone.verificationStatus, a technical mirror
// field kept in sync by routes.VerifyDomainDNS's cascade.
package entities

import (
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/reconciler"
	"github.com/metap-org/waap/model"
)

func DomainEntity() metadata.EntityDefinition {
	apexDomain := metadata.NewField(model.FieldApexDomain, "Apex Domain", metadata.KindString).Require().Index()
	apexDomain.Unique = metadata.Ptr(true)
	// A domain belongs to one tenant platform-wide, not once per tenant.
	globalScope := metadata.UniqueScopeGlobal
	apexDomain.UniqueScope = &globalScope
	apexDomain.Searchable = metadata.Ptr(true)

	return metadata.EntityDefinition{
		Name:      model.Domains,
		Label:     "Domain",
		TableName: reconciler.QualifiedTableNameIn(model.Domains, "waf"),
		Fields: []metadata.EntityField{
			apexDomain,
			// Auto-generated server-side (guardedbackend.resolveDomainID) when a zone's hostname
			// resolves to a new apex domain — never entered by hand.
			metadata.NewField(model.FieldVerificationToken, "Verification Token", metadata.KindString).Require(),
			metadata.EnumField(model.FieldVerificationMethod, "Verification Method", model.VerificationMethodDNSTxt, "httpFile").Require().Index(),
			metadata.EnumField(model.FieldVerificationStatus, "Verification Status", model.VerificationUnverified, model.VerificationVerified).Require().Index(),
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields: []string{"apexDomain", "verificationStatus"}, Filters: []string{"apexDomain", "verificationStatus"},
			MaxLimit: 100,
		}},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
		Actions: []metadata.EntityAction{
			{Name: model.ActionVerifyDNS, Label: "Verify domain ownership", Description: "Checks the _waf-verify TXT record and cascades onto the domain's zones.", Scope: metadata.ActionScopeRecord},
		},
	}
}

func init() { metadata.SubmitEntity(DomainEntity) }
