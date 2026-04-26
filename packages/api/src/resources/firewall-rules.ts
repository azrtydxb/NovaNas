import { type FirewallRule, FirewallRuleSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildFirewallRuleResource(db: DbClient): PgResource<FirewallRule> {
  return new PgResource<FirewallRule>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'FirewallRule',
    schema: FirewallRuleSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<FirewallRule>({
    app,
    basePath: '/api/v1/firewall-rules',
    tag: 'firewall-rules',
    kind: 'FirewallRule',
    resource: buildFirewallRuleResource(db),
    schema: FirewallRuleSchema,
  });
}
