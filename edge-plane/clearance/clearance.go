// Package clearance implements the challenge clearance cookie: proof that a client already
// passed a challenge recently.
//
// A keyed hash of (zone, client ip, expiry), so a cookie cannot be moved to another IP or
// another zone and cannot be extended by editing the expiry — the expiry is inside the hashed
// input. No server-side state, which is what lets any node in a fleet honour a clearance issued
// by any other node (given a shared ClearanceSecret).
//
// Strength, stated honestly: SHA-256 over secret||fields is not HMAC, and this whole mechanism
// is only as strong as "an attacker has not read this file and does not have the secret". It
// raises the cost of a trivial scripted flood, which is what the v1 challenge action is for. Bot
// management proper is v2; if this ever needs to resist a motivated attacker, it needs a real
// HMAC and a real challenge, not a tweak here.
package clearance

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func nowSecs() uint64 {
	return uint64(time.Now().Unix())
}

func sign(secret, zoneID, clientIP string, expiresAt uint64) string {
	h := sha256.New()
	h.Write([]byte(secret))
	h.Write([]byte("|"))
	h.Write([]byte(zoneID))
	h.Write([]byte("|"))
	h.Write([]byte(clientIP))
	h.Write([]byte("|"))
	h.Write([]byte(strconv.FormatUint(expiresAt, 10)))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Issue builds the Set-Cookie value handed out with a challenge page.
//
// HttpOnly so page scripts can't read it, SameSite=Lax so it survives a normal navigation back
// to the site, and Path=/ because a challenge clears the visitor for the whole zone.
func Issue(cookieName, secret, zoneID, clientIP string, ttl time.Duration) string {
	expiresAt := nowSecs() + uint64(ttl.Seconds())
	signature := sign(secret, zoneID, clientIP, expiresAt)
	return fmt.Sprintf("%s=%d.%s; Path=/; Max-Age=%d; HttpOnly; SameSite=Lax",
		cookieName, expiresAt, signature, int64(ttl.Seconds()))
}

// Verify reports whether the cookie this client presented is a valid, unexpired clearance for
// this zone and IP.
func Verify(cookieHeader, cookieName, secret, zoneID, clientIP string) bool {
	value, ok := cookieValue(cookieHeader, cookieName)
	if !ok {
		return false
	}
	expiresRaw, signature, found := strings.Cut(value, ".")
	if !found {
		return false
	}
	expiresAt, err := strconv.ParseUint(expiresRaw, 10, 64)
	if err != nil {
		return false
	}
	if expiresAt <= nowSecs() {
		return false
	}
	// Comparing the full hex string; a timing side channel on a challenge clearance is not a
	// meaningful threat next to the fact that this is not an HMAC to begin with (package doc).
	return sign(secret, zoneID, clientIP, expiresAt) == signature
}

// cookieValue pulls one cookie out of a Cookie: header without a cookie-parsing dependency.
func cookieValue(header, name string) (string, bool) {
	for _, pair := range strings.Split(header, ";") {
		pair = strings.TrimSpace(pair)
		key, value, found := strings.Cut(pair, "=")
		if found && key == name {
			return value, true
		}
	}
	return "", false
}
