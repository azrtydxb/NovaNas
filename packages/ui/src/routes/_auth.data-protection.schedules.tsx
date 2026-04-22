import { createFileRoute } from '@tanstack/react-router';
import { CalendarClock, Plus } from 'lucide-react';
import { ShellScreen } from '@/components/common/shell-screen';
import { Button } from '@/components/ui/button';

export const Route = createFileRoute('/_auth/data-protection/schedules')({
  component: SchedulesPage,
});

function SchedulesPage() {
  return (
    <ShellScreen
      title='Snapshot schedules'
      subtitle='Recurring snapshots with tiered retention.'
      icon={<CalendarClock size={28} />}
      actions={
        <Button variant='primary'>
          <Plus size={13} /> New schedule
        </Button>
      }
      upcoming={[
        'Schedule list with cron, retention and dataset scope',
        'Next/last run and produced snapshot counts',
        'Dry-run preview showing which snapshots would be pruned',
        'Pause/resume and on-demand trigger',
      ]}
    />
  );
}
