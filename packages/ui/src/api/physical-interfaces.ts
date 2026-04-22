import type { PhysicalInterface } from '@novanas/schemas';
import { useQuery } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export const physicalInterfacesKey = () => ['physical-interfaces'] as const;

export function usePhysicalInterfaces() {
  return useQuery<PhysicalInterface[]>({
    queryKey: physicalInterfacesKey(),
    queryFn: async () => unwrapList<PhysicalInterface>(await api.get('/physical-interfaces')),
    ...QUERY_DEFAULTS,
  });
}
