/** Whether an address is one that many unrelated clients share, so letting it through lets all of
 *  them through: carrier-grade NAT (100.64.0.0/10) or a private range. Only IPv4 is recognised; an
 *  IPv6 address or a CIDR is never flagged (the caller decides what to do about those). */
export function isSharedAddress(ip: string): boolean {
  const m = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(ip.trim());
  if (!m) return false;
  const [a, b] = [Number(m[1]), Number(m[2])];
  if (a === 100 && b >= 64 && b <= 127) return true;
  return (
    a === 10 || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168)
  );
}
