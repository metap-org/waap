package proxy

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForwardBuffersABodyWithinTheLimit(t *testing.T) {
	var got string
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
	}))
	defer origin.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/x", strings.NewReader("hello"))
	require.NoError(t, Forward(BuildClient(), rec, req, origin.URL, "203.0.113.1", "example.com", 5))
	assert.Equal(t, "hello", got)
}

func TestForwardRefusesABodyOverTheLimitWithoutReachingTheOrigin(t *testing.T) {
	reached := false
	origin := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
	defer origin.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/x", strings.NewReader("hello!"))
	err := Forward(BuildClient(), rec, req, origin.URL, "203.0.113.1", "example.com", 5)

	var perr *Error
	require.True(t, errors.As(err, &perr))
	assert.Equal(t, ErrBodyTooLarge, perr.Kind)
	assert.False(t, reached, "an oversized body must be refused before the origin is called")
}
