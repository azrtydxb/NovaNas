import { Redis } from 'ioredis';
import type { Env } from '../env.js';

export function createRedisClient(env: Env): Redis {
  return new Redis(env.REDIS_URL, {
    lazyConnect: false,
    maxRetriesPerRequest: 3,
    enableReadyCheck: true,
  });
}
