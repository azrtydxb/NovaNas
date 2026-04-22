import { randomBytes } from 'node:crypto';
import type { Redis } from 'ioredis';
import type { SessionRecord } from '../types.js';

/**
 * Redis-backed session store. Sessions have a 1-hour sliding TTL —
 * every successful request resets the TTL.
 */
export const SESSION_TTL_SECONDS = 60 * 60; // 1 hour
const KEY_PREFIX = 'session:';

export function newSessionId(): string {
  return randomBytes(32).toString('base64url');
}

export class SessionStore {
  constructor(private readonly redis: Redis) {}

  private key(id: string): string {
    return `${KEY_PREFIX}${id}`;
  }

  async create(record: SessionRecord): Promise<string> {
    const id = newSessionId();
    await this.redis.setex(this.key(id), SESSION_TTL_SECONDS, JSON.stringify(record));
    return id;
  }

  async get(id: string): Promise<SessionRecord | null> {
    const raw = await this.redis.get(this.key(id));
    if (!raw) return null;
    try {
      return JSON.parse(raw) as SessionRecord;
    } catch {
      return null;
    }
  }

  /** Get and refresh TTL in a single round-trip. */
  async touch(id: string): Promise<SessionRecord | null> {
    const record = await this.get(id);
    if (!record) return null;
    await this.redis.expire(this.key(id), SESSION_TTL_SECONDS);
    return record;
  }

  async destroy(id: string): Promise<void> {
    await this.redis.del(this.key(id));
  }
}
