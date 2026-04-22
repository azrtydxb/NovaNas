import { type AuditQuery, useAuditSearch } from '@/api/audit';
import { EmptyState } from '@/components/common/empty-state';
import { FormField } from '@/components/common/form-field';
import { PageHeader } from '@/components/common/page-header';
import { StatusDot } from '@/components/common/status-dot';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeaderCell,
  TableRow,
} from '@/components/ui/table';
import { i18n } from '@/lib/i18n';
import { Trans } from '@lingui/react';
import { createFileRoute } from '@tanstack/react-router';
import { ScrollText } from 'lucide-react';
import { useState } from 'react';

export const Route = createFileRoute('/_auth/system/audit')({
  component: AuditPage,
});

function AuditPage() {
  const [actor, setActor] = useState('');
  const [kind, setKind] = useState('');
  const [outcome, setOutcome] = useState<'all' | 'ok' | 'warn' | 'err'>('all');
  const [since, setSince] = useState('');
  const [until, setUntil] = useState('');
  const [applied, setApplied] = useState<AuditQuery>({});

  const q = useAuditSearch(applied);

  const apply = () => {
    setApplied({
      actor: actor || undefined,
      kind: kind || undefined,
      outcome: outcome === 'all' ? undefined : outcome,
      since: since || undefined,
      until: until || undefined,
      limit: 200,
    });
  };

  const clear = () => {
    setActor('');
    setKind('');
    setOutcome('all');
    setSince('');
    setUntil('');
    setApplied({});
  };

  return (
    <>
      <PageHeader
        title={i18n._('Audit log')}
        subtitle={i18n._('System and user actions, filterable.')}
      />
      <section className='grid grid-cols-5 gap-3 mb-3'>
        <FormField label={i18n._('Actor')}>
          <Input value={actor} onChange={(e) => setActor(e.target.value)} placeholder='alice' />
        </FormField>
        <FormField label={i18n._('Kind')}>
          <Input value={kind} onChange={(e) => setKind(e.target.value)} placeholder='Dataset' />
        </FormField>
        <FormField label={i18n._('Outcome')}>
          <Select value={outcome} onValueChange={(v) => setOutcome(v as typeof outcome)}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='all'>{i18n._('all')}</SelectItem>
              <SelectItem value='ok'>ok</SelectItem>
              <SelectItem value='warn'>warn</SelectItem>
              <SelectItem value='err'>err</SelectItem>
            </SelectContent>
          </Select>
        </FormField>
        <FormField label={i18n._('Since (ISO)')}>
          <Input
            value={since}
            onChange={(e) => setSince(e.target.value)}
            placeholder='2026-01-01'
          />
        </FormField>
        <FormField label={i18n._('Until (ISO)')}>
          <Input value={until} onChange={(e) => setUntil(e.target.value)} />
        </FormField>
      </section>
      <div className='flex gap-2 mb-3'>
        <Button variant='primary' onClick={apply}>
          <Trans id='Apply filters' />
        </Button>
        <Button variant='ghost' onClick={clear}>
          <Trans id='Clear' />
        </Button>
      </div>

      {q.isLoading ? (
        <Skeleton className='h-40' />
      ) : q.isError ? (
        <EmptyState
          icon={<ScrollText size={28} />}
          title={i18n._('Unable to load audit log')}
          description={(q.error as Error)?.message}
          action={<Button onClick={() => q.refetch()}>{i18n._('Retry')}</Button>}
        />
      ) : (q.data?.length ?? 0) === 0 ? (
        <EmptyState icon={<ScrollText size={28} />} title={i18n._('No matching events.')} />
      ) : (
        <div className='border border-border rounded-md overflow-hidden'>
          <Table>
            <TableHead>
              <tr>
                <TableHeaderCell>
                  <Trans id='Time' />
                </TableHeaderCell>
                <TableHeaderCell>
                  <Trans id='Actor' />
                </TableHeaderCell>
                <TableHeaderCell>
                  <Trans id='Verb' />
                </TableHeaderCell>
                <TableHeaderCell>
                  <Trans id='Resource' />
                </TableHeaderCell>
                <TableHeaderCell>
                  <Trans id='Message' />
                </TableHeaderCell>
              </tr>
            </TableHead>
            <TableBody>
              {q.data!.map((e) => (
                <TableRow key={e.id}>
                  <TableCell className='mono text-xs whitespace-nowrap'>{e.timestamp}</TableCell>
                  <TableCell className='mono text-xs'>{e.actor}</TableCell>
                  <TableCell className='mono text-xs'>
                    <StatusDot
                      tone={
                        e.tone === 'err'
                          ? 'err'
                          : e.tone === 'warn'
                            ? 'warn'
                            : e.tone === 'ok'
                              ? 'ok'
                              : 'idle'
                      }
                      className='mr-2'
                    />
                    {e.verb}
                  </TableCell>
                  <TableCell className='mono text-xs'>
                    {e.resource
                      ? `${e.resource}${e.resourceName ? `/${e.resourceName}` : ''}`
                      : '—'}
                  </TableCell>
                  <TableCell className='text-xs'>{e.message}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </>
  );
}
