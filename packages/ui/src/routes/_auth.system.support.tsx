import { api } from '@/api/client';
import { EmptyState } from '@/components/common/empty-state';
import { PageHeader } from '@/components/common/page-header';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeaderCell,
  TableRow,
} from '@/components/ui/table';
import { useAuth } from '@/hooks/use-auth';
import { useToast } from '@/hooks/use-toast';
import { i18n } from '@/lib/i18n';
import { Trans } from '@lingui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { createFileRoute } from '@tanstack/react-router';
import { Download, LifeBuoy } from 'lucide-react';

interface SupportBundle {
  id: string;
  createdAt: string;
  sizeBytes: number;
  status: 'generating' | 'ready' | 'failed';
  downloadUrl?: string;
}

export const Route = createFileRoute('/_auth/system/support')({
  component: SupportPage,
});

function SupportPage() {
  const { canMutate } = useAuth();
  const qc = useQueryClient();
  const toast = useToast();
  const bundles = useQuery<SupportBundle[]>({
    queryKey: ['support-bundles'],
    queryFn: async () => {
      const res = await api.get<{ items?: SupportBundle[] } | SupportBundle[]>(
        '/system/support-bundles'
      );
      return Array.isArray(res) ? res : (res?.items ?? []);
    },
  });

  const generate = useMutation({
    mutationFn: () => api.post<SupportBundle>('/system/support-bundles', {}),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['support-bundles'] }),
  });

  const mayMutate = canMutate();

  return (
    <>
      <PageHeader
        title={i18n._('Support')}
        subtitle={i18n._('Generate diagnostic bundles for support.')}
        actions={
          mayMutate ? (
            <Button
              variant='primary'
              onClick={async () => {
                try {
                  await generate.mutateAsync();
                  toast.success(i18n._('Support bundle queued'));
                } catch (e) {
                  toast.error(i18n._('Failed to queue bundle'), (e as Error).message);
                }
              }}
              disabled={generate.isPending}
            >
              {generate.isPending ? <Trans id='Queuing…' /> : <Trans id='Generate bundle' />}
            </Button>
          ) : null
        }
      />

      {bundles.isLoading ? (
        <Skeleton className='h-24' />
      ) : bundles.isError ? (
        <EmptyState
          icon={<LifeBuoy size={28} />}
          title={i18n._('Unable to load bundles')}
          description={(bundles.error as Error)?.message}
          action={<Button onClick={() => bundles.refetch()}>{i18n._('Retry')}</Button>}
        />
      ) : (bundles.data?.length ?? 0) === 0 ? (
        <EmptyState
          icon={<LifeBuoy size={28} />}
          title={i18n._('No support bundles yet')}
          description={i18n._(
            'Generate one to collect diagnostics, configuration, and recent logs.'
          )}
        />
      ) : (
        <div className='border border-border rounded-md overflow-hidden'>
          <Table>
            <TableHead>
              <tr>
                <TableHeaderCell>
                  <Trans id='ID' />
                </TableHeaderCell>
                <TableHeaderCell>
                  <Trans id='Created' />
                </TableHeaderCell>
                <TableHeaderCell>
                  <Trans id='Size' />
                </TableHeaderCell>
                <TableHeaderCell>
                  <Trans id='Status' />
                </TableHeaderCell>
                <TableHeaderCell className='text-right'>
                  <Trans id='Download' />
                </TableHeaderCell>
              </tr>
            </TableHead>
            <TableBody>
              {bundles.data!.map((b) => (
                <TableRow key={b.id}>
                  <TableCell className='mono text-xs'>{b.id}</TableCell>
                  <TableCell className='mono text-xs'>{b.createdAt}</TableCell>
                  <TableCell className='mono text-xs'>
                    {(b.sizeBytes / 1e6).toFixed(1)} MB
                  </TableCell>
                  <TableCell className='text-xs'>{b.status}</TableCell>
                  <TableCell className='text-right'>
                    {b.status === 'ready' && b.downloadUrl ? (
                      <a href={b.downloadUrl} download>
                        <Button size='sm' variant='ghost'>
                          <Download size={12} />
                        </Button>
                      </a>
                    ) : (
                      <span className='text-foreground-subtle text-xs'>—</span>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </>
  );
}
