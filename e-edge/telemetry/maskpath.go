package telemetry

import (
	"regexp"
	"strings"
)

// A request path leaves the edge masked (BR-44: personal data and tokens are never logged). Only the
// telemetry copy changes: rules and the origin still see the path as it arrived.
//
// This is recognition by shape, so it is a floor, not a guarantee: a short secret, or one made of
// letters only, in a place nothing here recognizes, passes through.

const (
	maskEmail  = "{email}"
	maskUUID   = "{uuid}"
	maskNumber = "{number}"
	maskToken  = "{token}"
	maskValue  = "{value}"
)

var (
	uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	jwtRe  = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*`)
	// keyRe: API keys that announce themselves by prefix.
	keyRe  = regexp.MustCompile(`^(sk_|sk-|pk_|rk_|ghp_|gho_|ghs_|github_pat_|glpat-|xox[abprs]-|AKIA|AIza)[A-Za-z0-9_-]{8,}`)
	dateRe = regexp.MustCompile(`^\d{4}[-.]\d{2}[-.]\d{2}$`)
	// numberRe: digits with the separators people write phone, identity, account and card numbers with.
	numberRe = regexp.MustCompile(`^\+?[0-9][0-9 .-]*$`)
	hexRe    = regexp.MustCompile(`^[0-9a-fA-F]{16,}$`)
)

// secretFollows are segments after which a link usually carries its secret.
var secretFollows = map[string]bool{
	"reset": true, "reset-password": true, "reset_password": true, "password-reset": true,
	"token": true, "verify": true, "confirm": true, "invite": true, "invitation": true,
	"activate": true, "magic": true, "magic-link": true, "unsubscribe": true, "session": true,
	"apikey": true, "api-key": true, "secret": true, "otp": true, "code": true,
}

// MaskPath replaces the parts of a decoded URL path that look personal or secret.
func MaskPath(path string) string {
	segments := strings.Split(path, "/")
	previous := ""
	for i, segment := range segments {
		if segment == "" {
			continue
		}
		base, params, hasParams := strings.Cut(segment, ";")
		masked := maskSegment(base, secretFollows[previous])
		if hasParams {
			masked += ";" + maskParams(params)
		}
		segments[i] = masked
		previous = strings.ToLower(base)
	}
	return strings.Join(segments, "/")
}

// maskParams keeps the names of matrix parameters (";jsessionid=...") and drops their values.
func maskParams(params string) string {
	parts := strings.Split(params, ";")
	for i, part := range parts {
		if name, _, found := strings.Cut(part, "="); found {
			parts[i] = name + "=" + maskValue
		} else {
			parts[i] = maskSegment(part, false)
		}
	}
	return strings.Join(parts, ";")
}

func maskSegment(s string, secretExpected bool) string {
	switch {
	case s == "":
		return s
	case isEmail(s):
		return maskEmail
	case uuidRe.MatchString(s):
		return maskUUID
	case jwtRe.MatchString(s), keyRe.MatchString(s):
		return maskToken
	case numberRe.MatchString(s) && countDigits(s) >= 8 && !dateRe.MatchString(s):
		return maskNumber
	case looksOpaque(s):
		return maskToken
	case secretExpected && (len(s) >= 16 || (len(s) >= 6 && countDigits(s) > 0)):
		return maskToken
	}
	return s
}

func isEmail(s string) bool {
	local, domain, found := strings.Cut(s, "@")
	return found && local != "" && strings.Contains(domain, ".")
}

// looksOpaque: a long run of mixed characters that is not a readable slug. A slug is short words
// joined by -, _ or .; a generated value has a long unbroken part, or mixes both letter cases with
// digits.
func looksOpaque(s string) bool {
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' || r == '.' }) {
		if hexRe.MatchString(part) && countDigits(part) > 0 {
			return true
		}
		if len(part) >= 20 && countDigits(part) > 0 && countDigits(part) < len(part) {
			return true
		}
	}
	return len(s) >= 24 && countDigits(s) >= 2 &&
		strings.ToLower(s) != s && strings.ToUpper(s) != s && !strings.ContainsAny(s, " .")
}

func countDigits(s string) int {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	return n
}
