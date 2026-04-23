import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerSmbServers } from '../resources/smb-servers.js';
import { registerUnavailable } from './_unavailable.js';

export async function smbServerRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerSmbServers(app, api);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/smb-servers', summary: 'List SMB servers', tag: 'smb-servers' },
    {
      method: 'POST',
      url: '/api/v1/smb-servers',
      summary: 'Create an SMB server',
      tag: 'smb-servers',
    },
    {
      method: 'GET',
      url: '/api/v1/smb-servers/:name',
      summary: 'Get an SMB server',
      tag: 'smb-servers',
    },
    {
      method: 'PATCH',
      url: '/api/v1/smb-servers/:name',
      summary: 'Update an SMB server',
      tag: 'smb-servers',
    },
    {
      method: 'DELETE',
      url: '/api/v1/smb-servers/:name',
      summary: 'Delete an SMB server',
      tag: 'smb-servers',
    },
  ]);
}
