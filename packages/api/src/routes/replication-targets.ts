import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/replication-targets.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function replicationTargetsRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/replication-targets', summary: 'List ReplicationTargets', tag: 'replication-targets' },
    { method: 'POST', url: '/api/v1/replication-targets', summary: 'Create a ReplicationTarget', tag: 'replication-targets' },
    { method: 'GET', url: '/api/v1/replication-targets/:name', summary: 'Get a ReplicationTarget', tag: 'replication-targets' },
    { method: 'PATCH', url: '/api/v1/replication-targets/:name', summary: 'Update a ReplicationTarget', tag: 'replication-targets' },
    { method: 'DELETE', url: '/api/v1/replication-targets/:name', summary: 'Delete a ReplicationTarget', tag: 'replication-targets' },
  ]);
}
