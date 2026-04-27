import type { FastifyInstance } from 'fastify';
import { register as registerBackendAssignments } from '../resources/backend-assignments.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function backendAssignmentsRoutes(
  app: FastifyInstance,
  db?: DbClient | null
): Promise<void> {
  if (db) {
    registerBackendAssignments(app, db);
    return;
  }
  registerUnavailable(app, [
    {
      method: 'GET',
      url: '/api/v1/backend-assignments',
      summary: 'List BackendAssignments',
      tag: 'backend-assignments',
    },
    {
      method: 'POST',
      url: '/api/v1/backend-assignments',
      summary: 'Create a BackendAssignment',
      tag: 'backend-assignments',
    },
    {
      method: 'GET',
      url: '/api/v1/backend-assignments/:name',
      summary: 'Get a BackendAssignment',
      tag: 'backend-assignments',
    },
    {
      method: 'PATCH',
      url: '/api/v1/backend-assignments/:name',
      summary: 'Update a BackendAssignment',
      tag: 'backend-assignments',
    },
    {
      method: 'DELETE',
      url: '/api/v1/backend-assignments/:name',
      summary: 'Delete a BackendAssignment',
      tag: 'backend-assignments',
    },
  ]);
}
