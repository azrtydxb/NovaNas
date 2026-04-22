/**
 * GpuDevice API hooks (issue #5).
 *
 * B2-API-Routes will expose `/api/v1/gpu-devices` (REST list + a targeted
 * `assign` action). Until then the list call will 404 and the UI will render
 * the "No GPUs detected" empty state.
 */
import { useLiveQuery } from '@/hooks/use-live-query';
import type { GpuDevice } from '@novanas/schemas';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export const gpuDevicesKey = () => ['gpu-devices'] as const;
export const gpuDeviceKey = (name: string) => ['gpu-device', name] as const;

export function useGpuDevices(options?: { enabled?: boolean }) {
  const enabled = options?.enabled ?? true;
  return useLiveQuery<GpuDevice[]>(
    gpuDevicesKey(),
    async () => unwrapList<GpuDevice>(await api.get('/gpu-devices')),
    {
      ...QUERY_DEFAULTS,
      staleTime: 60_000,
      enabled,
      wsChannel: enabled ? 'gpudevice:*' : null,
    }
  );
}

export function useGpuDevice(name: string | undefined) {
  return useLiveQuery<GpuDevice>(
    gpuDeviceKey(name ?? ''),
    () => api.get<GpuDevice>(`/gpu-devices/${encodeURIComponent(name!)}`),
    {
      ...QUERY_DEFAULTS,
      staleTime: 60_000,
      enabled: !!name,
      wsChannel: name ? `gpudevice:${name}` : null,
    }
  );
}

export interface GpuAssignBody {
  vm?: string;
  app?: string;
}

export function useAssignGpuDevice(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: GpuAssignBody) =>
      api.post(`/gpu-devices/${encodeURIComponent(name)}/assign`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: gpuDevicesKey() });
      qc.invalidateQueries({ queryKey: gpuDeviceKey(name) });
    },
  });
}
