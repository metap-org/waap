// Boot sequence for billing-service — plans and subscriptions (UC-70, UC-74), and the answer to
// "what may this tenant use?" that zones-service asks before it creates a zone or rule. Owns
// waf.plans and waf.subscriptions, which live in the operator's tenant (OPERATOR_TENANT_ID).
// Same single-database topology as the other WAAP services; tokens are verified through JWKS_URL.
package main

import (
	"github.com/metap-org/metap/pkg/app"

	"github.com/metap-org/waap/m-billing"
	// Declared entities register themselves in init(); app.Main reconciles them, referenced first.
	_ "github.com/metap-org/waap/m-billing/entities"
)

func main() {
	app.Main(app.Options{
		Name:    "billing-service",
		Routes:  billingservice.MountRoutes,
		Actions: billingservice.RegisterActions,
	})
}
