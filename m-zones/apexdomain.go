// The apex-domain grouping algorithm: the last 2 labels of a hostname, split on "." — no Public
// Suffix List, so a compound TLD like ".co.uk" groups on "co.uk" rather than the real apex.
// Accepted trade-off at this scale, not a bug to fix later without a real PSL dependency.
package zonesservice

import "strings"

// ApexDomain: a hostname with fewer than 2 labels (a bare TLD, or already just 2 labels) returns
// itself unchanged — there's nothing shorter to group on.
func ApexDomain(hostname string) string {
	labels := strings.Split(strings.TrimRight(hostname, "."), ".")
	if len(labels) <= 2 {
		return hostname
	}
	return strings.Join(labels[len(labels)-2:], ".")
}
