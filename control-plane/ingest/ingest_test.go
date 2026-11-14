package ingest

import (
	"context"
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

func TestRunWriterSendsOneBatchPerDrain(t *testing.T) {
	sink := portsmock.NewEventSink(t)
	queue := make(chan EdgeEvent, 10)
	for i := 0; i < 3; i++ {
		queue <- EdgeEvent{ZoneID: "z1", Action: "blocked", SourceIP: "203.0.113.7"}
	}
	done := make(chan struct{})
	sink.EXPECT().WriteSecurityEvents(mock.Anything, mock.MatchedBy(func(events []map[string]any) bool {
		return len(events) == 3 && events[0]["zoneId"] == "z1" && events[0]["sourceIp"] == "203.0.113.7"
	})).RunAndReturn(func(context.Context, []map[string]any) (int, error) {
		close(done)
		return 1, nil
	}).Once()

	shutdown := make(chan struct{})
	go RunWriter(context.Background(), sink, queue, 200, shutdown)
	<-done
	close(shutdown)
}

func TestIngestRefusesABodyOverTheByteLimitBeforeReadingItAll(t *testing.T) {
	h := Router(newState(t, 10, nil))
	big := `{"events":[{"zoneId":"` + strings.Repeat("z", int(maxIngestBodyBytes)) + `"}]}`
	rec := post(t, h, "", big)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Contains(t, rec.Body.String(), `"code":"payload_too_large"`)
}

func TestIngestErrorsUseTheStandardEnvelope(t *testing.T) {
	h := Router(newState(t, 10, nil))
	rec := post(t, h, "", `{"events":[{},{},{},{}]}`)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.JSONEq(t, `{"error":{"code":"too_many_events","message":"at most 3 events per request"}}`, rec.Body.String())
	assert.Equal(t, http.StatusBadRequest, post(t, h, "", `{`).Code)
}
