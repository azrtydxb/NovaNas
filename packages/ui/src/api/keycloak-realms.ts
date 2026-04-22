import type { KeycloakRealm, KeycloakRealmSpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type KeycloakRealmCreateBody = { metadata: { name: string }; spec: KeycloakRealmSpec };
export type KeycloakRealmUpdateBody = { spec: Partial<KeycloakRealmSpec> };

export const keycloakRealmsKey = () => ['keycloak-realms'] as const;
export const keycloakRealmKey = (name: string) => ['keycloak-realm', name] as const;

export function useKeycloakRealms() {
  return useQuery<KeycloakRealm[]>({
    queryKey: keycloakRealmsKey(),
    queryFn: async () => unwrapList<KeycloakRealm>(await api.get('/keycloak-realms')),
    ...QUERY_DEFAULTS,
  });
}

export function useCreateKeycloakRealm() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: KeycloakRealmCreateBody) =>
      api.post<KeycloakRealm>('/keycloak-realms', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: keycloakRealmsKey() }),
  });
}

export function useUpdateKeycloakRealm(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: KeycloakRealmUpdateBody) =>
      api.patch<KeycloakRealm>(`/keycloak-realms/${encodeURIComponent(name)}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: keycloakRealmsKey() });
      qc.invalidateQueries({ queryKey: keycloakRealmKey(name) });
    },
  });
}

export function useDeleteKeycloakRealm() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/keycloak-realms/${encodeURIComponent(name)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: keycloakRealmsKey() }),
  });
}
