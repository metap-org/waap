package netcheck_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/metap-org/metap/pkg/action/actiontest"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/waap/m-management/zones/netcheck"
	"github.com/metap-org/waap/model"
)

// dohAnswering is a DNS-over-HTTPS resolver that answers every TXT question with txt.
func dohAnswering(t *testing.T, txt ...string) netcheck.Config {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		answers := []map[string]string{}
		for _, v := range txt {
			answers = append(answers, map[string]string{"data": `"` + v + `"`})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Answer": answers})
	}))
	t.Cleanup(srv.Close)
	return netcheck.Config{DoHURL: srv.URL}
}

// domainFixture is a domain record with two zones under it, as the action finds them.
type domainFixture struct {
	id      uuid.UUID
	zones   []uuid.UUID
	records *actiontest.Records
}

func newDomain(token string) domainFixture {
	f := domainFixture{id: uuid.New(), zones: []uuid.UUID{uuid.New(), uuid.New()}, records: &actiontest.Records{}}
	f.records.Store(model.Domains, f.id, map[string]any{"apexDomain": "example.com", "verificationToken": token})
	f.records.Listed = map[string][]crud.RecordDto{model.Zones: {
		{ID: f.zones[0], Entity: model.Zones, Version: 4}, {ID: f.zones[1], Entity: model.Zones, Version: 7},
	}}
	return f
}

func (f domainFixture) run(cfg netcheck.Config, input map[string]any) actiontest.Result {
	call := actiontest.Call(model.Domains, model.ActionVerifyDNS, f.id, input, actiontest.Caller("analyst"))
	return actiontest.RunWith(netcheck.VerifyDomain(cfg), call, f.records, &actiontest.Sink{})
}

func TestVerifyDomainMarksTheDomainAndItsZonesWhenTheTokenIsPublished(t *testing.T) {
	f := newDomain("tok-123")
	res := f.run(dohAnswering(t, "tok-123"), nil)
	require.NoError(t, res.Err)

	assert.JSONEq(t, `true`, string(res.Out["ownershipVerified"]))
	require.Len(t, res.Records.Writes, 3, "the domain, then each of its zones")
	assert.Equal(t, model.Domains, res.Records.Writes[0].Entity)
	assert.Equal(t, f.id, res.Records.Writes[0].ID)
	assert.JSONEq(t, `"verified"`, string(res.Records.Writes[0].Data[model.FieldVerificationStatus]))
	assert.JSONEq(t, `"dnsTxt"`, string(res.Records.Writes[0].Data[model.FieldVerificationMethod]))
	assert.ElementsMatch(t, f.zones, []uuid.UUID{res.Records.Writes[1].ID, res.Records.Writes[2].ID})

	require.Len(t, res.Sink.Entries, 1, "the verification is audited")
	assert.Equal(t, "domain.ownership_verified", *res.Sink.Entries[0].TransitionAction)
}

func TestVerifyDomainWritesNothingWhenTheTokenIsNotThere(t *testing.T) {
	f := newDomain("tok-123")
	res := f.run(dohAnswering(t, "someone-elses-token"), nil)
	require.NoError(t, res.Err)

	assert.JSONEq(t, `false`, string(res.Out["ownershipVerified"]))
	assert.Empty(t, res.Records.Writes)
	assert.Empty(t, res.Sink.Entries)
	assert.Contains(t, res.Out, "domain", "the answer still carries the domain as it is")
}

func TestVerifyDomainNeverTrustsAnEmptyToken(t *testing.T) {
	f := newDomain("") // the domain has no verification token
	res := f.run(dohAnswering(t, ""), nil)
	require.NoError(t, res.Err)
	assert.JSONEq(t, `false`, string(res.Out["ownershipVerified"]))
	assert.Empty(t, res.Records.Writes)
}

// The caller cannot say which token to look for: a TXT value already published under _waf-verify
// would otherwise prove a domain.
func TestVerifyDomainRefusesATokenFromTheCaller(t *testing.T) {
	f := newDomain("tok-123")
	res := f.run(dohAnswering(t, "someone-elses-token"), map[string]any{"expectedToken": "someone-elses-token"})
	se, ok := crud.AsServiceError(res.Err)
	require.True(t, ok)
	assert.EqualValues(t, http.StatusBadRequest, se.Status)
	assert.Empty(t, res.Records.Writes)
}

func TestVerifyDomainRefusesAMethodItDoesNotKnow(t *testing.T) {
	f := newDomain("tok-123")
	res := f.run(dohAnswering(t), map[string]any{"method": "carrierPigeon"})
	se, ok := crud.AsServiceError(res.Err)
	require.True(t, ok)
	assert.EqualValues(t, http.StatusUnprocessableEntity, se.Status)
	assert.Contains(t, se.FieldErrors, "method")
	assert.Empty(t, res.Records.Writes)
}

func TestVerifyDomainRefusesAnInputFieldItDoesNotHave(t *testing.T) {
	f := newDomain("tok-123")
	res := f.run(dohAnswering(t, "tok-123"), map[string]any{"methd": "dnsTxt"})
	se, ok := crud.AsServiceError(res.Err)
	require.True(t, ok)
	assert.EqualValues(t, http.StatusBadRequest, se.Status)
	assert.Empty(t, res.Records.Writes)
}

func TestVerifyDomainRefusesADomainWithoutAnApexDomain(t *testing.T) {
	f := newDomain("tok-123")
	f.records.Store(model.Domains, f.id, map[string]any{"verificationToken": "tok-123"})
	res := f.run(dohAnswering(t, "tok-123"), nil)
	se, ok := crud.AsServiceError(res.Err)
	require.True(t, ok)
	assert.EqualValues(t, http.StatusBadRequest, se.Status)
}

// One zone that cannot be updated must not stop the others, and must not undo the domain.
func TestVerifyDomainKeepsGoingWhenOneZoneCannotBeMarked(t *testing.T) {
	f := newDomain("tok-123")
	f.records.FailWrite = func(w actiontest.Write) error {
		if w.ID == f.zones[0] {
			return errors.New("zone is locked")
		}
		return nil
	}
	res := f.run(dohAnswering(t, "tok-123"), nil)
	require.NoError(t, res.Err, "the domain is verified even though one zone could not be marked")
	assert.JSONEq(t, `true`, string(res.Out["ownershipVerified"]))

	var written []uuid.UUID
	for _, w := range res.Records.Writes {
		written = append(written, w.ID)
	}
	assert.ElementsMatch(t, []uuid.UUID{f.id, f.zones[1]}, written)
}

func TestVerifyDomainFailsAsAWholeWhenTheDomainItselfCannotBeUpdated(t *testing.T) {
	f := newDomain("tok-123")
	f.records.FailWrite = func(w actiontest.Write) error {
		if w.Entity == model.Domains {
			return crud.NewServiceError(http.StatusConflict, "version_conflict")
		}
		return nil
	}
	res := f.run(dohAnswering(t, "tok-123"), nil)
	se, ok := crud.AsServiceError(res.Err)
	require.True(t, ok)
	assert.EqualValues(t, http.StatusConflict, se.Status)
	assert.Empty(t, res.Sink.Entries, "nothing is audited for a verification that did not happen")
}
