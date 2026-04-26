// Minimal OpenBao (HashiCorp Vault-compatible) admin client.
//
// Built for the inlined operator side effects on #51 — at present
// only KmsKey needs Transit-engine operations (key create/update/
// delete). Auth is the static `OPENBAO_TOKEN` from env; if either
// OPENBAO_ADDR or OPENBAO_TOKEN is missing the factory returns null
// and the resource hooks become no-ops.

import type { Env } from '../env.js';

export interface EnsureTransitKeyOptions {
  /** Cipher type. Defaults to aes256-gcm96. */
  type?: string;
  /** Allow `bao key delete` without first toggling deletion_allowed. */
  deletionAllowed?: boolean;
  /** Permit raw key export. Almost never wanted; default false. */
  exportable?: boolean;
  /** Allow ciphertext backups that include unsealed key material. */
  allowPlaintextBackup?: boolean;
  /** Auto-rotation period (e.g. "720h"). Bao expects a Go duration. */
  autoRotatePeriod?: string;
}

export interface OpenBaoAdmin {
  /**
   * Create the Transit key if it does not already exist; otherwise
   * patch its mutable configuration. Returns true if a new key was
   * created, false if an existing one was reconfigured.
   */
  ensureTransitKey(name: string, opts?: EnsureTransitKeyOptions): Promise<boolean>;
  /**
   * Best-effort delete. Toggles `deletion_allowed=true` first if the
   * key still has it disabled, then issues the actual delete. Missing
   * key is not an error.
   */
  deleteTransitKey(name: string): Promise<void>;
  /**
   * Generate a fresh data key wrapped by the named Transit key. Returns
   * the ciphertext (the "wrapped DK") and the key version that produced
   * it. The plaintext is intentionally NOT returned — callers persist
   * only the ciphertext, and recover the plaintext later via decrypt
   * at the consumer site.
   */
  generateDataKey(keyName: string): Promise<{ ciphertext: string; keyVersion: number }>;
}

export function createOpenBaoAdmin(env: Env): OpenBaoAdmin | null {
  if (!env.OPENBAO_ADDR || !env.OPENBAO_TOKEN) return null;
  const baseUrl = env.OPENBAO_ADDR.replace(/\/$/, '');
  const token = env.OPENBAO_TOKEN;

  async function bao(path: string, init: RequestInit = {}): Promise<Response> {
    return fetch(`${baseUrl}${path}`, {
      ...init,
      headers: {
        ...(init.headers ?? {}),
        'X-Vault-Token': token,
      },
    });
  }

  async function transitKeyExists(name: string): Promise<boolean> {
    const res = await bao(`/v1/transit/keys/${encodeURIComponent(name)}`);
    if (res.status === 404) return false;
    if (!res.ok) {
      throw new Error(
        `openbao-admin: transit key ${name} lookup failed (${res.status}): ${await res.text()}`
      );
    }
    return true;
  }

  return {
    async ensureTransitKey(name: string, opts: EnsureTransitKeyOptions = {}): Promise<boolean> {
      const exists = await transitKeyExists(name);
      if (!exists) {
        const create = await bao(`/v1/transit/keys/${encodeURIComponent(name)}`, {
          method: 'POST',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({
            type: opts.type ?? 'aes256-gcm96',
            exportable: opts.exportable ?? false,
            allow_plaintext_backup: opts.allowPlaintextBackup ?? false,
            // Auto-rotate is set on create (config endpoint can change
            // it later but combining the two saves a round trip).
            ...(opts.autoRotatePeriod ? { auto_rotate_period: opts.autoRotatePeriod } : {}),
          }),
        });
        if (!create.ok && create.status !== 204) {
          throw new Error(
            `openbao-admin: transit key ${name} create failed (${create.status}): ${await create.text()}`
          );
        }
      }
      // Apply (or re-apply) the mutable config in either branch — the
      // create call above doesn't accept deletion_allowed, and we want
      // updates to be idempotent on existing keys.
      const config: Record<string, unknown> = {};
      if (opts.deletionAllowed !== undefined) config.deletion_allowed = opts.deletionAllowed;
      if (opts.allowPlaintextBackup !== undefined)
        config.allow_plaintext_backup = opts.allowPlaintextBackup;
      if (opts.autoRotatePeriod) config.auto_rotate_period = opts.autoRotatePeriod;
      if (Object.keys(config).length > 0) {
        const patch = await bao(`/v1/transit/keys/${encodeURIComponent(name)}/config`, {
          method: 'POST',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify(config),
        });
        if (!patch.ok && patch.status !== 204) {
          throw new Error(
            `openbao-admin: transit key ${name} config failed (${patch.status}): ${await patch.text()}`
          );
        }
      }
      return !exists;
    },

    async generateDataKey(keyName: string): Promise<{ ciphertext: string; keyVersion: number }> {
      // /transit/datakey/wrapped/{name} is the variant that returns
      // ONLY the ciphertext (no plaintext). That's what we want here —
      // the api never sees the plaintext DK, the agent recovers it at
      // mount time via /transit/decrypt.
      const res = await bao(`/v1/transit/datakey/wrapped/${encodeURIComponent(keyName)}`, {
        method: 'POST',
      });
      if (!res.ok) {
        throw new Error(
          `openbao-admin: generateDataKey ${keyName} failed (${res.status}): ${await res.text()}`
        );
      }
      const body = (await res.json()) as {
        data: { ciphertext: string; key_version: number };
      };
      return { ciphertext: body.data.ciphertext, keyVersion: body.data.key_version };
    },

    async deleteTransitKey(name: string): Promise<void> {
      if (!(await transitKeyExists(name))) return;
      // Bao refuses delete unless deletion_allowed=true; flip it
      // first. If the caller set deletionProtection, the hook caller
      // is responsible for not invoking deleteTransitKey at all.
      const flip = await bao(`/v1/transit/keys/${encodeURIComponent(name)}/config`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ deletion_allowed: true }),
      });
      if (!flip.ok && flip.status !== 204) {
        throw new Error(
          `openbao-admin: transit key ${name} flip-deletion failed (${flip.status}): ${await flip.text()}`
        );
      }
      const del = await bao(`/v1/transit/keys/${encodeURIComponent(name)}`, { method: 'DELETE' });
      if (!del.ok && del.status !== 204 && del.status !== 404) {
        throw new Error(
          `openbao-admin: transit key ${name} delete failed (${del.status}): ${await del.text()}`
        );
      }
    },
  };
}
