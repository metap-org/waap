module github.com/metap-org/waap/edge-plane

go 1.26.10

require (
	github.com/redis/go-redis/v9 v9.23.0
	github.com/stretchr/testify v1.12.1
)

require go.yaml.in/yaml/v3 v3.0.5 // indirect

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/metap-org/metap v0.0.0
	go.uber.org/atomic v1.12.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
)

// Only github.com/metap-org/metap/pkg/kit/* may be imported from metap (standard-library-only
// utilities) — enforced by depguard, see .golangci.yml. Local sibling checkout, same convention
// as data-plane/go and control-plane/go.
replace github.com/metap-org/metap => ../../metap
