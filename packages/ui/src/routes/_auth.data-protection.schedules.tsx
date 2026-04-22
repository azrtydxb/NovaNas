import { ShellScreen } from '@/components/common/shell-screen';
import { Button } from '@/components/ui/button';
import { i18n } from '@/lib/i18n';
import { Trans } from '@lingui/react';
import { createFileRoute } from '@tanstack/react-router';
import { CalendarClock, Plus } from 'lucide-react';

export const Route = createFileRoute('/_auth/data-protection/schedules')({
  component: SchedulesPage,
});

function SchedulesPage() {
  return (
    <ShellScreen
      title={i18n._('Snapshot schedules')}
      subtitle={i18n._('Recurring snapshots with tiered retention.')}
      icon={<CalendarClock size={28} />}
      actions={
        <Button variant='primary'>
          <Plus size={13} /> <Trans id='New schedule' />
        </Button>
      }
      upcoming={[
        i18n._('Schedule list with cron, retention and dataset scope'),
        i18n._('Next/last run and produced snapshot counts'),
        i18n._('Dry-run preview showing which snapshots would be pruned'),
        i18n._('Pause/resume and on-demand trigger'),
      ]}
    />
  );
}
