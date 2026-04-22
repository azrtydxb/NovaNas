import { i18n } from '@/lib/i18n';
import { Outlet, useRouterState } from '@tanstack/react-router';
import { Sidebar } from './sidebar';
import { Topbar } from './topbar';

const LABELS: Record<string, string> = {
  '/dashboard': 'Dashboard',
  '/storage/pools': 'Pools',
  '/storage/datasets': 'Datasets',
  '/storage/disks': 'Disks',
  '/storage/snapshots': 'Snapshots',
  '/sharing/shares': 'Shares',
  '/sharing/iscsi': 'iSCSI / NVMe-oF',
  '/sharing/s3': 'S3',
  '/data-protection/schedules': 'Snapshot schedules',
  '/data-protection/replication': 'Replication',
  '/data-protection/cloud-backup': 'Cloud Backup',
  '/apps': 'Apps',
  '/vms': 'Virtual Machines',
  '/network': 'Network',
  '/identity/users': 'Users',
  '/identity/groups': 'Groups',
  '/system/settings': 'Settings',
  '/system/updates': 'Updates',
  '/system/certificates': 'Certificates',
  '/system/alerts': 'Alerts',
  '/system/jobs': 'Jobs',
  '/system/audit': 'Audit log',
  '/system/support': 'Support',
};

export function Chrome() {
  const { location } = useRouterState();
  // i18n-wave-12: run the static label through the i18n runtime so languages
  // other than en can pick up a translation once catalogs are generated.
  const rawTitle = LABELS[location.pathname] ?? 'Console';
  const title = i18n._(rawTitle);
  return (
    <div className='grid grid-cols-[232px_1fr] grid-rows-[48px_1fr] h-screen min-h-0'>
      <Topbar currentPageTitle={title} />
      <Sidebar />
      <main className='overflow-auto p-4 pb-10 px-5'>
        <Outlet />
      </main>
    </div>
  );
}
