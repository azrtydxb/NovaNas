export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  return `${(n / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

// SHA-256 fingerprint of a PEM-encoded public key. Matches what the
// backend would emit if it derived a fingerprint server-side: hex of
// the SHA-256 digest of the *raw PEM bytes* (including BEGIN/END
// armor). Returned as `sha256:<64-hex>` so it's distinguishable from
// other algorithms.
export async function pemFingerprint(pem: string): Promise<string> {
  const buf = new TextEncoder().encode(pem);
  const digest = await crypto.subtle.digest("SHA-256", buf);
  const hex = Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
  return `sha256:${hex}`;
}
