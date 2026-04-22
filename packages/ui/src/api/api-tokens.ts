import type { ApiToken, ApiTokenSpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type ApiTokenCreateBody = { metadata: { name: string }; spec: ApiTokenSpec };

export interface ApiTokenCreateResponse {
  token: ApiToken;
  /** Plaintext secret returned ONCE on creation. */
  secret: string;
}

export const apiTokensKey = () => ['api-tokens'] as const;

export function useApiTokens() {
  return useQuery<ApiToken[]>({
    queryKey: apiTokensKey(),
    queryFn: async () => unwrapList<ApiToken>(await api.get('/api-tokens')),
    ...QUERY_DEFAULTS,
  });
}

export function useCreateApiToken() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: ApiTokenCreateBody) => api.post<ApiTokenCreateResponse>('/api-tokens', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: apiTokensKey() }),
  });
}

export function useRevokeApiToken() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/api-tokens/${encodeURIComponent(name)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: apiTokensKey() }),
  });
}
