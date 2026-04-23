import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerNfsServers } from '../resources/nfs-servers.js';
import { registerUnavailable } from './_unavailable.js';

export async function nfsServerRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerNfsServers(app, api);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/nfs-servers', summary: 'List NFS servers', tag: 'nfs-servers' },
    {
      method: 'POST',
      url: '/api/v1/nfs-servers',
      summary: 'Create an NFS server',
      tag: 'nfs-servers',
    },
    {
      method: 'GET',
      url: '/api/v1/nfs-servers/:name',
      summary: 'Get an NFS server',
      tag: 'nfs-servers',
    },
    {
      method: 'PATCH',
      url: '/api/v1/nfs-servers/:name',
      summary: 'Update an NFS server',
      tag: 'nfs-servers',
    },
    {
      method: 'DELETE',
      url: '/api/v1/nfs-servers/:name',
      summary: 'Delete an NFS server',
      tag: 'nfs-servers',
    },
  ]);
}
