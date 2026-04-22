import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type FirewallRule, FirewallRuleSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildFirewallRuleResource(api: CustomObjectsApi): CrdResource<FirewallRule> {
  return new CrdResource<FirewallRule>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'firewallrules' },
    schema: FirewallRuleSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<FirewallRule>({
    app,
    basePath: '/api/v1/firewall-rules',
    tag: 'firewall-rules',
    kind: 'FirewallRule',
    resource: buildFirewallRuleResource(api),
    schema: FirewallRuleSchema,
  });
}
