import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/smb-servers.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function smbServerRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/smb-servers', summary: 'List SmbServers', tag: 'smb-servers' },
    {
      method: 'POST',
      url: '/api/v1/smb-servers',
      summary: 'Create a SmbServer',
      tag: 'smb-servers',
    },
    {
      method: 'GET',
      url: '/api/v1/smb-servers/:name',
      summary: 'Get a SmbServer',
      tag: 'smb-servers',
    },
    {
      method: 'PATCH',
      url: '/api/v1/smb-servers/:name',
      summary: 'Update a SmbServer',
      tag: 'smb-servers',
    },
    {
      method: 'DELETE',
      url: '/api/v1/smb-servers/:name',
      summary: 'Delete a SmbServer',
      tag: 'smb-servers',
    },
  ]);
}
