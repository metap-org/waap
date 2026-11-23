package alerting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIncidentTitleNamesTheWholeAddress(t *testing.T) {
	title := "12 events from 11.2.3.45 in 15m"
	assert.True(t, incidentTitleNames(title, "11.2.3.45"))
	assert.False(t, incidentTitleNames(title, "1.2.3.4"), "a prefix or suffix of another address is not the same address")
	assert.False(t, incidentTitleNames(title, "2.3.4"))
	assert.True(t, incidentTitleNames("7 events from 2001:db8::1 in 30m", "2001:db8::1"))
	assert.False(t, incidentTitleNames("7 events from 2001:db8::10 in 30m", "2001:db8::1"))
}
