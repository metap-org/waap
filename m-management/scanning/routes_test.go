package scanning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/metap-org/metap/pkg/kit/httpx/router"
	"github.com/metap-org/metap/pkg/web"
	"github.com/metap-org/waap/m-management/scanning"
)

// A route written by hand is not covered by metap's own route inventory: nothing resolves its caller
// unless it was registered through an access gate. This is that inventory for this service's routes.
func TestRoutesHaveAnAccessRule(t *testing.T) {
	r := router.New()
	scanning.MountRoutes(r, &web.State{})
	assert.NotEmpty(t, r.Routes())
	assert.Empty(t, web.RoutesWithoutOneAccessRule(r))
}
