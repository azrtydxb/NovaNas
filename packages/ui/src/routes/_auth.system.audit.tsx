import { createFileRoute } from '@tanstack/react-router';
import { ScrollText } from 'lucide-react';
import { ShellScreen } from '@/components/common/shell-screen';

export const Route = createFileRoute('/_auth/system/audit')({
  component: AuditPage,
});

function AuditPage() {
  return (
    <ShellScreen
      title='Audit log'
      subtitle='Every state change, WHO / WHAT / WHEN / WHY.'
      icon={<ScrollText size={28} />}
      upcoming={[
        'Filterable audit stream by actor, object and action',
        'Full context: request, decision, downstream effects',
        'Export to S3 / syslog via AuditPolicy sinks',
        'Retention and redaction controls',
      ]}
    />
  );
}
