import type { FirewallRule, FirewallRuleSpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type FirewallRuleCreateBody = { metadata: { name: string }; spec: FirewallRuleSpec };
export type FirewallRuleUpdateBody = { spec: Partial<FirewallRuleSpec> };

export const firewallRulesKey = () => ['firewall-rules'] as const;
export const firewallRuleKey = (name: string) => ['firewall-rule', name] as const;

export function useFirewallRules() {
  return useQuery<FirewallRule[]>({
    queryKey: firewallRulesKey(),
    queryFn: async () => unwrapList<FirewallRule>(await api.get('/firewall-rules')),
    ...QUERY_DEFAULTS,
  });
}

export function useCreateFirewallRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: FirewallRuleCreateBody) => api.post<FirewallRule>('/firewall-rules', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: firewallRulesKey() }),
  });
}

export function useUpdateFirewallRule(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: FirewallRuleUpdateBody) =>
      api.patch<FirewallRule>(`/firewall-rules/${encodeURIComponent(name)}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: firewallRulesKey() });
      qc.invalidateQueries({ queryKey: firewallRuleKey(name) });
    },
  });
}

export function useDeleteFirewallRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/firewall-rules/${encodeURIComponent(name)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: firewallRulesKey() }),
  });
}
