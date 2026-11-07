package ingest

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/metap-org/waap/control-plane/ports/portsmock"
)

func post(t *testing.T, h http.Handler, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/ingest/events", strings.NewReader(body))
	if token != "" {
		req.Header.Set("x-waf-ingest-token", token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func newState(t *testing.T, queue int, token *string) *State {
	return &State{Queue: make(chan EdgeEvent, queue), Distributor: portsmock.NewRuleSink(t), MaxRequestEvents: 3, IngestToken: token}
}

const twoEvents = `{"events":[{"zoneId":"z1","action":"block"},{"zoneId":"z1","action":"log"}]}`

func TestIngestRejectsAMissingOrWrongToken(t *testing.T) {
	token := "s3cret"
	h := Router(newState(t, 10, &token))
	assert.Equal(t, http.StatusUnauthorized, post(t, h, "", twoEvents).Code)
	assert.Equal(t, http.StatusUnauthorized, post(t, h, "s3cre", twoEvents).Code)
	assert.Equal(t, http.StatusUnauthorized, post(t, h, "s3cret-and-more", twoEvents).Code)
	assert.Equal(t, http.StatusAccepted, post(t, h, "s3cret", twoEvents).Code)
}

func TestIngestQueuesEventsAndDropsRatherThanBlockingWhenFull(t *testing.T) {
	state := newState(t, 1, nil)
	rec := post(t, Router(state), "", twoEvents)
	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.JSONEq(t, `{"data":{"accepted":1,"dropped":1}}`, rec.Body.String())
	assert.Len(t, state.Queue, 1)
}

func TestIngestRejectsMalformedAndOversizedBatches(t *testing.T) {
	h := Router(newState(t, 10, nil))
	assert.Equal(t, http.StatusBadRequest, post(t, h, "", `{not json`).Code)
	four := `{"events":[{},{},{},{}]}`
	assert.Equal(t, http.StatusRequestEntityTooLarge, post(t, h, "", four).Code)
}

func TestHealthReportsRedisReachability(t *testing.T) {
	for name, tc := range map[string]struct {
		pingErr error
		status  int
	}{"up": {nil, http.StatusOK}, "down": {errors.New("refused"), http.StatusServiceUnavailable}} {
		t.Run(name, func(t *testing.T) {
			sink := portsmock.NewRuleSink(t)
			sink.EXPECT().Ping(mock.Anything).Return(tc.pingErr)
			rec := httptest.NewRecorder()
			Router(&State{Distributor: sink}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
			assert.Equal(t, tc.status, rec.Code)
		})
	}
}
