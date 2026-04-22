import type { AlertChannel, AlertChannelSpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type AlertChannelCreateBody = { metadata: { name: string }; spec: AlertChannelSpec };
export type AlertChannelUpdateBody = { spec: Partial<AlertChannelSpec> };

export const alertChannelsKey = () => ['alert-channels'] as const;

export function useAlertChannels() {
  return useQuery<AlertChannel[]>({
    queryKey: alertChannelsKey(),
    queryFn: async () => unwrapList<AlertChannel>(await api.get('/alert-channels')),
    ...QUERY_DEFAULTS,
  });
}

export function useCreateAlertChannel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: AlertChannelCreateBody) => api.post<AlertChannel>('/alert-channels', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: alertChannelsKey() }),
  });
}

export function useUpdateAlertChannel(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: AlertChannelUpdateBody) =>
      api.patch<AlertChannel>(`/alert-channels/${encodeURIComponent(name)}`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: alertChannelsKey() }),
  });
}

export function useDeleteAlertChannel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/alert-channels/${encodeURIComponent(name)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: alertChannelsKey() }),
  });
}
