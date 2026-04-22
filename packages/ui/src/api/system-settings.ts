import type { SystemSettings, SystemSettingsSpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api } from './client';

export const systemSettingsKey = () => ['system-settings'] as const;

export function useSystemSettings() {
  return useQuery<SystemSettings>({
    queryKey: systemSettingsKey(),
    queryFn: () => api.get<SystemSettings>('/system/settings'),
    ...QUERY_DEFAULTS,
  });
}

export function useUpdateSystemSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (spec: Partial<SystemSettingsSpec>) =>
      api.patch<SystemSettings>('/system/settings', { spec }),
    onSuccess: () => qc.invalidateQueries({ queryKey: systemSettingsKey() }),
  });
}
