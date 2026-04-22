import {
  useCertificates,
  useCreateCertificate,
  useDeleteCertificate,
  useRenewCertificate,
} from '@/api/certificates';
import { EmptyState } from '@/components/common/empty-state';
import { FormField } from '@/components/common/form-field';
import { PageHeader } from '@/components/common/page-header';
import { StatusDot } from '@/components/common/status-dot';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
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
import { useAuth } from '@/hooks/use-auth';
import { useToast } from '@/hooks/use-toast';
import { i18n } from '@/lib/i18n';
import { Trans } from '@lingui/react';
import { createFileRoute } from '@tanstack/react-router';
import { Lock, Plus, RefreshCcw, Trash2 } from 'lucide-react';
import { useState } from 'react';

export const Route = createFileRoute('/_auth/system/certificates')({
  component: CertificatesPage,
});

function CertificatesPage() {
  const { canMutate } = useAuth();
  const q = useCertificates();
  const renew = useRenewCertificate();
  const del = useDeleteCertificate();
  const toast = useToast();
  const [createOpen, setCreateOpen] = useState(false);
  const mayMutate = canMutate();

  return (
    <>
      <PageHeader
        title={i18n._('Certificates')}
        subtitle={i18n._('TLS certificates issued via ACME, internal PKI, or uploaded.')}
        actions={
          mayMutate ? (
            <Button variant='primary' onClick={() => setCreateOpen(true)}>
              <Plus size={13} /> <Trans id='Request certificate' />
            </Button>
          ) : null
        }
      />
      {q.isLoading ? (
        <Skeleton className='h-24' />
      ) : q.isError ? (
        <EmptyState
          icon={<Lock size={28} />}
          title={i18n._('Unable to load certificates')}
          description={(q.error as Error)?.message}
          action={<Button onClick={() => q.refetch()}>{i18n._('Retry')}</Button>}
        />
      ) : (q.data?.length ?? 0) === 0 ? (
        <EmptyState
          icon={<Lock size={28} />}
          title={i18n._('No certificates yet')}
          action={
            mayMutate ? (
              <Button variant='primary' onClick={() => setCreateOpen(true)}>
                <Plus size={13} /> <Trans id='Request certificate' />
              </Button>
            ) : undefined
          }
        />
      ) : (
        <div className='border border-border rounded-md overflow-hidden'>
          <Table>
            <TableHead>
              <tr>
                <TableHeaderCell>
                  <Trans id='Name' />
                </TableHeaderCell>
                <TableHeaderCell>
                  <Trans id='Common name' />
                </TableHeaderCell>
                <TableHeaderCell>
                  <Trans id='Provider' />
                </TableHeaderCell>
                <TableHeaderCell>
                  <Trans id='Phase' />
                </TableHeaderCell>
                <TableHeaderCell>
                  <Trans id='Not after' />
                </TableHeaderCell>
                <TableHeaderCell className='text-right'>
                  <Trans id='Actions' />
                </TableHeaderCell>
              </tr>
            </TableHead>
            <TableBody>
              {q.data!.map((c) => (
                <TableRow key={c.metadata.name}>
                  <TableCell>
                    <StatusDot
                      tone={
                        c.status?.phase === 'Issued'
                          ? 'ok'
                          : c.status?.phase === 'Failed' || c.status?.phase === 'Expired'
                            ? 'err'
                            : 'warn'
                      }
                      className='mr-2'
                    />
                    <span className='mono text-xs'>{c.metadata.name}</span>
                  </TableCell>
                  <TableCell className='mono text-xs'>{c.spec.commonName}</TableCell>
                  <TableCell>
                    <Badge>{c.spec.provider}</Badge>
                  </TableCell>
                  <TableCell className='text-xs'>{c.status?.phase ?? 'Pending'}</TableCell>
                  <TableCell className='mono text-xs'>{c.status?.notAfter ?? '—'}</TableCell>
                  <TableCell className='text-right'>
                    {mayMutate && (
                      <div className='flex gap-1 justify-end'>
                        <Button
                          size='sm'
                          variant='ghost'
                          onClick={async () => {
                            try {
                              await renew.mutateAsync(c.metadata.name);
                              toast.success(i18n._('Renewal requested'));
                            } catch (e) {
                              toast.error(i18n._('Renew failed'), (e as Error).message);
                            }
                          }}
                        >
                          <RefreshCcw size={12} />
                        </Button>
                        <Button
                          size='sm'
                          variant='danger'
                          onClick={async () => {
                            try {
                              await del.mutateAsync(c.metadata.name);
                              toast.success(i18n._('Deleted'), c.metadata.name);
                            } catch (e) {
                              toast.error(i18n._('Delete failed'), (e as Error).message);
                            }
                          }}
                        >
                          <Trash2 size={12} />
                        </Button>
                      </div>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
      <CreateCertificateDialog open={createOpen} onOpenChange={setCreateOpen} />
    </>
  );
}

function CreateCertificateDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreateCertificate();
  const toast = useToast();
  const [name, setName] = useState('');
  const [provider, setProvider] = useState<'acme' | 'internalPki' | 'upload'>('acme');
  const [commonName, setCommonName] = useState('');
  const [dnsNames, setDnsNames] = useState('');
  const [acmeIssuer, setAcmeIssuer] = useState<
    'letsencrypt' | 'letsencrypt-staging' | 'zerossl' | 'custom'
  >('letsencrypt');
  const [acmeEmail, setAcmeEmail] = useState('');

  const submit = async () => {
    if (!name || !commonName) {
      toast.error(i18n._('Missing fields'));
      return;
    }
    try {
      await create.mutateAsync({
        metadata: { name },
        spec: {
          provider,
          commonName,
          dnsNames: dnsNames
            ? dnsNames
                .split(',')
                .map((s) => s.trim())
                .filter(Boolean)
            : undefined,
          acme:
            provider === 'acme' ? { issuer: acmeIssuer, email: acmeEmail || undefined } : undefined,
        },
      });
      toast.success(i18n._('Certificate requested'), name);
      setName('');
      setCommonName('');
      setDnsNames('');
      setAcmeEmail('');
      onOpenChange(false);
    } catch (e) {
      toast.error(i18n._('Create failed'), (e as Error).message);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            <Trans id='Request certificate' />
          </DialogTitle>
          <DialogDescription>
            <Trans id='ACME is the easiest; internal PKI avoids DNS.' />
          </DialogDescription>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          <FormField label={i18n._('Name')} required>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </FormField>
          <FormField label={i18n._('Provider')}>
            <Select value={provider} onValueChange={(v) => setProvider(v as typeof provider)}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='acme'>ACME</SelectItem>
                <SelectItem value='internalPki'>Internal PKI</SelectItem>
                <SelectItem value='upload'>Upload</SelectItem>
              </SelectContent>
            </Select>
          </FormField>
          <FormField label={i18n._('Common name')} required>
            <Input value={commonName} onChange={(e) => setCommonName(e.target.value)} />
          </FormField>
          <FormField label={i18n._('DNS names')} hint={i18n._('Comma-separated SANs')}>
            <Input value={dnsNames} onChange={(e) => setDnsNames(e.target.value)} />
          </FormField>
          {provider === 'acme' && (
            <>
              <FormField label={i18n._('ACME issuer')}>
                <Select
                  value={acmeIssuer}
                  onValueChange={(v) => setAcmeIssuer(v as typeof acmeIssuer)}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='letsencrypt'>letsencrypt</SelectItem>
                    <SelectItem value='letsencrypt-staging'>letsencrypt-staging</SelectItem>
                    <SelectItem value='zerossl'>zerossl</SelectItem>
                    <SelectItem value='custom'>custom</SelectItem>
                  </SelectContent>
                </Select>
              </FormField>
              <FormField label={i18n._('Email')}>
                <Input
                  type='email'
                  value={acmeEmail}
                  onChange={(e) => setAcmeEmail(e.target.value)}
                />
              </FormField>
            </>
          )}
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            <Trans id='Cancel' />
          </Button>
          <Button variant='primary' onClick={submit} disabled={create.isPending}>
            {create.isPending ? <Trans id='Requesting…' /> : <Trans id='Request' />}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
