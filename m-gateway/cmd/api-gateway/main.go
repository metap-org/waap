// api-gateway is the Customer Portal's single HTTP front (roadmap 116; replaced waf-graphql-gateway):
// metap's generic pkg/apigateway, with nothing WAF-specific in it. Entity CRUD and declared
// actions (/api/{entity}...) go to the service that serves the entity, learned from each
// service's /metadata/entities; other routes go by prefix (UPSTREAMS). Every service verifies the
// caller's token itself.
//
//	LISTEN_ADDR  default 0.0.0.0:4000
//	UPSTREAMS    zones=http://zones-service:3000|/auth/,/me,/admin/,...;scanning=...;alerting=...
package main

import "github.com/metap-org/metap/pkg/apigateway"

func main() { apigateway.Main() }
