import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/replication-jobs.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function replicationJobsRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/replication-jobs', summary: 'List ReplicationJobs', tag: 'replication-jobs' },
    { method: 'POST', url: '/api/v1/replication-jobs', summary: 'Create a ReplicationJob', tag: 'replication-jobs' },
    { method: 'GET', url: '/api/v1/replication-jobs/:name', summary: 'Get a ReplicationJob', tag: 'replication-jobs' },
    { method: 'PATCH', url: '/api/v1/replication-jobs/:name', summary: 'Update a ReplicationJob', tag: 'replication-jobs' },
    { method: 'DELETE', url: '/api/v1/replication-jobs/:name', summary: 'Delete a ReplicationJob', tag: 'replication-jobs' },
  ]);
}
