import type { FastifyInstance, FastifyRequest } from 'fastify';
import { canWrite, ownNamespace } from '../auth/authz.js';
import { userFromClaims } from '../auth/rbac.js';
import type { SessionStore } from '../auth/session.js';
import type { Env } from '../env.js';
import { createSpiceProxy } from '../services/spice-proxy.js';

export interface VmConsoleDeps {
  env: Env;
  sessions: SessionStore;
  /** KubeVirt API server URL (e.g. https://kubernetes.default.svc). */
  apiServerUrl?: string;
  /** Bearer token for the API server (service account or user token). */
  apiServerToken?: string;
}

export async function vmConsoleRoutes(app: FastifyInstance, deps: VmConsoleDeps): Promise<void> {
  const { env, sessions, apiServerUrl, apiServerToken } = deps;

  app.get<{
    Params: { namespace: string; name: string };
    Querystring: { type?: 'spice' | 'vnc' };
  }>(
    '/api/v1/vms/:namespace/:name/console',
    { websocket: true },
    async (socket, req: FastifyRequest) => {
      // Authenticate the upgrade via session cookie.
      const raw = req.cookies?.[env.SESSION_COOKIE_NAME];
      if (!raw) {
        socket.close(4401, 'unauthorized');
        return;
      }
      const unsigned = req.unsignCookie(raw);
      if (!unsigned.valid || !unsigned.value) {
        socket.close(4401, 'unauthorized');
        return;
      }
      const session = await sessions.touch(unsigned.value);
      if (!session) {
        socket.close(4401, 'unauthorized');
        return;
      }
      const user = userFromClaims(session.claims);

      const { namespace, name } = (req.params ?? {}) as { namespace: string; name: string };
      const type = ((req.query ?? {}) as { type?: 'spice' | 'vnc' }).type ?? 'spice';

      // Mutation-level permission on VMs in that namespace.
      if (!canWrite(user, 'Vm', namespace)) {
        // fall back to own namespace check
        if (namespace !== ownNamespace(user)) {
          socket.close(4403, 'forbidden');
          return;
        }
      }

      if (!apiServerUrl) {
        socket.close(1011, 'upstream not configured');
        return;
      }

      req.log.info({ namespace, name, type, user: user.username }, 'vm.console.open');

      createSpiceProxy({
        vmNamespace: namespace,
        vmName: name,
        type,
        clientWs: socket as unknown as Parameters<typeof createSpiceProxy>[0]['clientWs'],
        apiServerUrl,
        apiServerToken,
        logger: req.log,
      });
    }
  );
}
