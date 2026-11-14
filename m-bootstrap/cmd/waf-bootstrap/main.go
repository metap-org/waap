// waf-bootstrap applies the WAAP personas (and an optional BOOTSTRAP_FILE overlay with initial
// users) to every active tenant, or to BOOTSTRAP_TENANT_IDS. One-shot and safe to re-run: it only
// adds what is missing. Passwords come from the environment variable each user's secretRef
// names; with NODE_ENV=production a missing one aborts before anything is written.
package main

import (
	"github.com/metap-org/metap/pkg/app"

	_ "github.com/metap-org/waap/m-alerting/entities"
	_ "github.com/metap-org/waap/m-bootstrap"
	_ "github.com/metap-org/waap/m-scanning/entities"
	_ "github.com/metap-org/waap/m-zones/entities"
)

func main() { app.BootstrapMain(app.BootstrapOptions{Name: "waf-bootstrap"}) }
