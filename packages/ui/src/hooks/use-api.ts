import { type RequestOptions, api } from '@/lib/api';
import {
  type UseMutationOptions,
  type UseQueryOptions,
  useMutation,
  useQuery,
} from '@tanstack/react-query';

export function useApiQuery<T>(
  key: readonly unknown[],
  path: string,
  options?: Omit<UseQueryOptions<T>, 'queryKey' | 'queryFn'> & { request?: RequestOptions }
) {
  const { request, ...rest } = options ?? {};
  return useQuery<T>({
    queryKey: key,
    queryFn: () => api.get<T>(path, request),
    ...rest,
  });
}

export function useApiMutation<TBody, TResp>(
  path: string | ((body: TBody) => string),
  method: 'POST' | 'PUT' | 'PATCH' | 'DELETE' = 'POST',
  options?: UseMutationOptions<TResp, unknown, TBody>
) {
  return useMutation<TResp, unknown, TBody>({
    mutationFn: async (body: TBody) => {
      const p = typeof path === 'function' ? path(body) : path;
      if (method === 'DELETE') return api.delete<TResp>(p);
      if (method === 'PUT') return api.put<TResp>(p, body);
      if (method === 'PATCH') return api.patch<TResp>(p, body);
      return api.post<TResp>(p, body);
    },
    ...options,
  });
}
